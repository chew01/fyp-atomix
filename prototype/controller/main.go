package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"prototype/controller/api"
	"prototype/controller/device"
	"prototype/controller/heartbeat"
	"prototype/controller/leadership"
	"prototype/controller/membership"
)

func main() {

	ctx, cancel := context.WithCancel(context.Background())
	log.Printf("a")
	defer cancel()

	hostname, _ := os.Hostname()
	log.Printf("b")
	electionManager := leadership.NewElectionManager(ctx, hostname)
	log.Printf("c")
	go device.Monitor(ctx, electionManager.StartElection, electionManager.StopElection)
	log.Printf("d")

	// Register as member
	membership.Register(ctx, hostname)
	log.Printf("e")
	go membership.Monitor(ctx)
	log.Printf("f")

	// Start heartbeat + monitor
	go heartbeat.StartHeartbeat(ctx, hostname, 5*time.Second)
	log.Printf("g")
	go heartbeat.MonitorHeartbeat(ctx, 15*time.Second, electionManager.StopAllElectionsForHostname)
	log.Printf("h")

	// Start HTTP server
	go api.StartServer(ctx, ":8080")

	// Wait for SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down...")
}
