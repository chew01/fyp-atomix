package device

import (
	"context"
	"fmt"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

type FakeDriver struct {
	ID string
}

func (f *FakeDriver) PushConfig(ctx context.Context, config map[string]string) error {
	driverMap, err := atomix.Map[string, string]("device").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device map: %w", err)
	}

	driverMap.Put(ctx, f.ID, fmt.Sprintf("%+v", config))
	return nil
}

func (f *FakeDriver) FetchStatus(ctx context.Context) (map[string]string, error) {
	driverMap, err := atomix.Map[string, string]("device").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get device map: %w", err)
	}

	entry, err := driverMap.Get(ctx, f.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device status: %w", err)
	}
	status := entry.Value

	return map[string]string{
		"uptime": time.Now().Format(time.RFC3339),
		"config": status,
	}, nil
}
