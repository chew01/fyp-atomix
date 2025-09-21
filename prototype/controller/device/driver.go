package device

import "context"

type Driver interface {
	PushConfig(ctx context.Context, config map[string]string) error
	FetchStatus(ctx context.Context) (map[string]string, error)
}
