package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"prototype/controller/election"
	"prototype/controller/membership"

	"github.com/atomix/go-sdk/pkg/atomix"
	election2 "github.com/atomix/go-sdk/pkg/primitive/election"
)

var devices = []string{"Switch-A", "Switch-B", "Switch-C"}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname, _ := os.Hostname()
	var elections []election2.Election

	// Start elections
	for _, device := range devices {
		e, err := atomix.LeaderElection("election-" + device).
			CandidateID(hostname).
			Get(ctx)
		if err != nil {
			log.Fatalf("[Election] (%s) Failed to create election: %v", device, err)
		}
		elections = append(elections, e)

		go election.RunElection(ctx, hostname, e)
	}

	// Start membership heartbeat + monitor
	membership.StartHeartbeat(ctx, hostname, 5*time.Second)
	membership.MonitorMembership(ctx, 15*time.Second, elections)

	// Wait for SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}
