package membership

import (
	"context"
	"io"
	"log"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
	"github.com/atomix/go-sdk/pkg/primitive/election"
)

func StartHeartbeat(ctx context.Context, hostname string, interval time.Duration) {
	membersMap, err := atomix.Map[string, int64]("members").
		Codec(generic.Scalar[int64]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Membership] Failed to get members map: %v", err)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ts := time.Now().Unix()
				_, err := membersMap.Put(ctx, hostname, ts)
				if err != nil {
					log.Printf("[Membership] Failed to heartbeat: %v", err)
				} else {
					log.Printf("[Membership] Heartbeat at %d", ts)
				}
			}
		}
	}()
}

func MonitorMembership(ctx context.Context, threshold time.Duration, elections []election.Election) {
	membersMap, err := atomix.Map[string, int64]("members").
		Codec(generic.Scalar[int64]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Membership] Failed to get members map: %v", err)
	}

	go func() {
		ticker := time.NewTicker(threshold / 2)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				entries, _ := membersMap.List(ctx)
				now := time.Now().Unix()
				count := 0

				for {
					entry, err := entries.Next()
					if err != nil {
						if err != io.EOF {
							log.Printf("[Membership] Iterator error: %v", err)
						}
						break
					}
					hostname, lastBeat := entry.Key, entry.Value
					count++
					if now-lastBeat > int64(threshold.Seconds()) {
						log.Printf("[Membership] ⚠️ Node %s offline (last seen %d)", hostname, lastBeat)
						_, _ = membersMap.Remove(ctx, hostname)
						for _, e := range elections {
							_, err := e.Evict(ctx, hostname)
							if err != nil {
								log.Printf("[Membership/Election] Failed to evict inactive node from election: %v", err)
							}
						}
					}
				}
				log.Printf("[Membership] Active nodes: %d", count)
			}
		}
	}()
}
