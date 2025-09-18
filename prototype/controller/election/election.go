package election

import (
	"context"
	"log"
	"reflect"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
	"github.com/atomix/go-sdk/pkg/primitive/election"
)

func RunElection(ctx context.Context, hostname string, e election.Election) {
	electionName := e.Name()
	// Join election
	term, err := e.Enter(ctx)
	if err != nil {
		log.Fatalf("[Election] (%s) Failed to enter election: %v", electionName, err)
	}
	log.Printf("[Election] (%s) Entered election: leader=%s, term=%d", electionName, term.Leader, term.ID)

	// Watch election
	stream, err := e.Watch(ctx)
	if err != nil {
		log.Printf("[Election] (%s) Failed to watch election: %v", electionName, err)
		return
	}

	// Distributed config map
	configMap, err := atomix.Map[string, string]("config").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Printf("[Election] (%s) Error accessing config map: %v", electionName, err)
	}

	var cache *election.Term
	for {
		term, err := stream.Next()
		if err != nil {
			log.Printf("[Election] (%s) Error in election stream: %v", electionName, err)
			time.Sleep(time.Second)
			continue
		}

		if cache == nil || cache.ID != term.ID {
			log.Printf("[Election] (%s) New term: %d", electionName, term.ID)
		}
		if cache == nil || !reflect.DeepEqual(cache.Candidates, term.Candidates) {
			log.Printf("[Election] (%s) Candidates: %v", electionName, term.Candidates)
		}

		if cache == nil || cache.Leader != term.Leader {
			if term.Leader == e.CandidateID() {
				log.Printf("[Election] (%s) ✅ I am leader (term %d)", electionName, term.ID)
				value := "leader " + hostname
				_, _ = configMap.Put(ctx, electionName, value)
			} else {
				log.Printf("[Election] (%s) ℹ️ Current leader: %s", electionName, term.Leader)
				if val, err := configMap.Get(ctx, e.Name()); err == nil {
					log.Printf("[Election] (%s) Follower sees: %s", electionName, val.Value)
				}
			}
		}
		cache = term
	}
}
