package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"prototype/controller/device"
	"prototype/controller/heartbeat"
	"prototype/controller/leadership"
	"prototype/controller/membership"

	"github.com/atomix/go-sdk/pkg/atomix"
	election "github.com/atomix/go-sdk/pkg/primitive/election"
)

var devices = []*device.Device{
	device.NewDevice("Switch-A", &device.FakeDriver{}),
	device.NewDevice("Switch-B", &device.FakeDriver{}),
	device.NewDevice("Switch-C", &device.FakeDriver{}),
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, dev := range devices {
		go dev.PollStatus(ctx)
	}

	hostname, _ := os.Hostname()
	var elections []election.Election

	// Start elections
	for _, device := range devices {
		e, err := atomix.LeaderElection("election-" + device.ID).
			CandidateID(hostname).
			Get(ctx)
		if err != nil {
			log.Fatalf("[Election] (%s) Failed to create election: %v", device, err)
		}
		elections = append(elections, e)

		go leadership.RunElection(ctx, hostname, e)
	}

	// Register as member
	membership.Register(ctx, hostname)
	go membership.Monitor(ctx)

	// Start heartbeat + monitor
	go heartbeat.StartHeartbeat(ctx, hostname, 5*time.Second)
	go heartbeat.MonitorHeartbeat(ctx, 15*time.Second, elections)

	// Wait for SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}
