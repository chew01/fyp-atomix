package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"prototype/controller/api"
	"prototype/controller/device"
	"prototype/controller/leadership"
	"prototype/controller/membership"
)

type Controller struct {
	Hostname          string
	ctx               context.Context
	ElectionManager   *leadership.ElectionManager
	MembershipManager *membership.MembershipManager
}

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostname, _ := os.Hostname()
	electionManager := leadership.NewElectionManager(ctx, hostname)
	go device.Monitor(ctx, electionManager.StartElection, electionManager.StopElection)

	membershipManager, err := membership.NewMembershipManager(ctx)
	if err != nil {
		log.Fatalf("Failed to create membership manager: %v", err)
	}
	go membershipManager.WatchControllers("name=prototype", electionManager.StopAllElectionsForHostname)

	// Start HTTP server
	go api.StartServer(ctx, membershipManager, ":8080")

	// Wait for SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}
