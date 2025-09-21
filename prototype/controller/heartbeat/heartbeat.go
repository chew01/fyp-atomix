package heartbeat

import (
	"context"
	"io"
	"log"
	"prototype/controller/membership"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
	"github.com/atomix/go-sdk/pkg/primitive/election"
)

func StartHeartbeat(ctx context.Context, hostname string, interval time.Duration) {
	heartbeatMap, err := atomix.Map[string, int64]("heartbeat").
		Codec(generic.Scalar[int64]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Heartbeat] Failed to get heartbeat map: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ts := time.Now().Unix()
			_, err := heartbeatMap.Put(ctx, hostname, ts)
			if err != nil {
				log.Printf("[Heartbeat] Failed to heartbeat: %v", err)
			} else {
				log.Printf("[Heartbeat] Heartbeat at %d", ts)
			}
		}
	}
}

func MonitorHeartbeat(ctx context.Context, threshold time.Duration, elections []election.Election) {
	heartbeatMap, err := atomix.Map[string, int64]("heartbeat").
		Codec(generic.Scalar[int64]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Heartbeat] Failed to get heartbeat map: %v", err)
	}

	ticker := time.NewTicker(threshold / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			entries, _ := heartbeatMap.List(ctx)
			now := time.Now().Unix()

			for {
				entry, err := entries.Next()
				if err != nil {
					if err != io.EOF {
						log.Printf("[Heartbeat] Iterator error: %v", err)
					}
					break
				}
				hostname, lastBeat := entry.Key, entry.Value
				if now-lastBeat > int64(threshold.Seconds()) { // Node is offline
					log.Printf("[Heartbeat] ⚠️ Node %s offline (last seen %d)", hostname, lastBeat)
					_, err := heartbeatMap.Remove(ctx, hostname)
					if err != nil {
						log.Printf("[Heartbeat] Failed to remove offline node from heartbeat map: %v", err)
					}
					membership.Remove(ctx, hostname)
					for _, e := range elections {
						_, err := e.Evict(ctx, hostname)
						if err != nil {
							log.Printf("[Heartbeat/Leadership] Failed to evict offline node from election: %v", err)
						}
					}
				}
			}
		}
	}
}
