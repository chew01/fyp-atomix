package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
	"github.com/atomix/go-sdk/pkg/primitive/election"
)

// Simple device list for PoC
var devices = []string{"Switch-A", "Switch-B", "Switch-C"}

func main() {
	ctx := context.Background()

	hostname, _ := os.Hostname()
	var elections []election.Election

	for _, dev := range devices {
		electionName := fmt.Sprintf("election-%s", dev)
		e, err := atomix.LeaderElection(electionName).CandidateID(hostname).Get(ctx)
		if err != nil {
			log.Fatalf("[%s] Failed to create leader election: %v", dev, err)
		}

		elections = append(elections, e)
		go participateInElection(ctx, hostname, dev, e)
	}
}

func participateInElection(ctx context.Context, hostname string, device string, e election.Election) {
	term, err := e.Enter(ctx)
	if err != nil {
		log.Fatalf("[%s] Failed to enter election: %v", device, err)
	}
	log.Printf("[%s] Entered election successfully as candidate %s (current leader: %s, term %d)", device, hostname, term.Leader, term.ID)

	stream, err := e.Watch(ctx)
	if err != nil {
		log.Printf("[%s] Failed to watch election: %v", device, err)
	}

	configMap, err := atomix.Map[string, string]("config").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		log.Printf("[%s] Error accessing configMap: %v", device, err)
	}

	var cache *election.Term

	for {
		term, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				log.Printf("[%s] Got EOF, terminating loop", device)
				return
			}
			log.Printf("[%s] Error in election stream: %v", device, err)
			time.Sleep(time.Second)
			continue
		}

		if cache == nil || cache.ID != term.ID {
			log.Printf("[%s] New election term: Term %d", device, term.ID)
		}

		if cache == nil || !reflect.DeepEqual(cache.Candidates, term.Candidates) {
			log.Printf("[%s] Election candidates updated: %v", device, term.Candidates)
		}

		if cache == nil || cache.Leader != term.Leader {
			if term.Leader == e.CandidateID() {
				log.Printf("[%s] ✅ I am now LEADER in term %d\n", device, term.ID)

				// Example: leader writes state into distributed map
				value := fmt.Sprintf("Leader %s updated state at %s", hostname, time.Now().Format(time.RFC3339))
				if _, err := configMap.Put(ctx, device, value); err != nil {
					log.Printf("[%s] Failed to write state: %v", device, err)
				} else {
					log.Printf("[%s] State updated in map: %s -> %s\n", device, device, value)
				}
			} else {
				log.Printf("[%s] ℹ️ Current leader: %s (term %d)\n", device, term.Leader, term.ID)

				// Followers can read the current state
				val, err := configMap.Get(ctx, device)
				if err == nil {
					log.Printf("[%s] Follower sees state: %s -> %s\n", device, device, val.Value)
				}
			}
		}

		cache = term
	}
}
