package main

import (
	"context"
	"log"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
)

func main() {
	ctx := context.Background()

	log.Println("Starting test")

	cnt, err := atomix.Counter("my-counter").Get(ctx)
	if err != nil {
		log.Fatalf("failed to create counter: %v", err)
	}
	log.Println("Counter created")

	err = cnt.Set(ctx, 10)
	if err != nil {
		log.Fatalf("failed to set counter: %v", err)
	}
	log.Println("Set counter: 10")

	count, err := cnt.Get(ctx)
	if err != nil {
		log.Fatalf("failed to get counter: %v", err)
	}
	log.Printf("Got counter: %v", count)

	// Sleep to keep the app running for observation
	time.Sleep(30 * time.Second)
}
