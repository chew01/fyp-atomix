package device

import (
	"context"
	"fmt"
	"log"
	"time"
)

type FakeDriver struct {
	lastConfig map[string]string
}

func (f *FakeDriver) PushConfig(ctx context.Context, config map[string]string) error {
	f.lastConfig = config
	log.Printf("[FakeDriver] Config pushed: %+v", config)
	return nil
}

func (f *FakeDriver) FetchStatus(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"uptime": time.Now().Format(time.RFC3339),
		"config": fmt.Sprintf("%+v", f.lastConfig),
	}, nil
}
