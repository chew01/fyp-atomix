package membership

import (
	"context"
	"io"
	"log"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

func Register(ctx context.Context, hostname string) {
	membershipSet, err := atomix.Set[string]("membership").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Membership] Failed to get membership set: %v", err)
	}
	defer membershipSet.Close(ctx)

	membershipSet.Add(ctx, hostname)
	log.Println("[Membership] Registered controller")
}

func Remove(ctx context.Context, hostname string) {
	membershipSet, err := atomix.Set[string]("membership").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Membership] Failed to get membership set: %v", err)
	}
	defer membershipSet.Close(ctx)

	membershipSet.Remove(ctx, hostname)
	log.Printf("[Membership] Removed member %s", hostname)
}

func Monitor(ctx context.Context) {
	membershipSet, err := atomix.Set[string]("membership").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Membership] Failed to get membership set: %v", err)
	}
	defer membershipSet.Close(ctx)

	stream, err := membershipSet.Watch(ctx)
	if err != nil {
		log.Printf("[Membership] Failed to watch membership set: %v", err)
		return
	}
	for {
		_, err := stream.Next()
		if err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded || err == io.EOF {
				return
			}
			log.Printf("[Membership] Error in membership stream: %v", err)
			continue
		}
		length, _ := membershipSet.Len(ctx)
		log.Printf("[Membership] Number of members: %d", length)
	}
}
