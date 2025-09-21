package device

import (
	"context"
	"log"
	"time"
)

type Device struct {
	ID     string
	Driver Driver
	State  map[string]string
}

func NewDevice(id string, driver Driver) *Device {
	return &Device{
		ID:     id,
		Driver: driver,
		State:  make(map[string]string),
	}
}

func (d *Device) ApplyConfig(ctx context.Context, config map[string]string) {
	log.Printf("[%s] Applying config: %+v", d.ID, config)
	d.State = config
	d.Driver.PushConfig(ctx, config)
}

func (d *Device) PollStatus(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			status, err := d.Driver.FetchStatus(ctx)
			if err != nil {
				log.Printf("[%s] Error fetching status: %v", d.ID, err)
				time.Sleep(5 * time.Second)
				continue
			}
			log.Printf("[%s] Status: %+v", d.ID, status)
			time.Sleep(5 * time.Second)
		}
	}
}
