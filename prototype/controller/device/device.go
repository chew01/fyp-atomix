package device

import (
	"context"
	"log"
	"time"
)

type Device struct {
	ID     string
	Driver Driver
}

func NewDevice(id string, driver Driver) *Device {
	return &Device{
		ID:     id,
		Driver: driver,
	}
}

func (d *Device) ApplyConfig(ctx context.Context, config map[string]string) {
	log.Printf("[%s] Applying config: %+v", d.ID, config)
	d.Driver.PushConfig(ctx, config)
}

func (d *Device) PollStatus(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			status, err := d.Driver.FetchStatus(ctx)
			if err != nil {
				// log.Printf("[%s] Error fetching status: %v", d.ID, err)
				time.Sleep(interval)
				continue
			}
			log.Printf("[%s] Status: %+v", d.ID, status)
			time.Sleep(interval)
		}
	}
}
