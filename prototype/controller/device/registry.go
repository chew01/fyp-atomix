package device

import (
	"context"
	"log"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

/*
In the current implementation, devices are registered in Atomix via HTTP server.
This helps us simulate without the use of actual devices or clients.
In real-world scenarios, devices will connect to controllers directly.
Therefore in that case, we will need a server for devices to connect to
as well as a controller function to register them to the device list.
That is TODO.
*/

func Monitor(ctx context.Context, start func(string, *Device), stop func(string)) {
	driverMap, err := atomix.Map[string, string]("device").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Devices] Failed to get device map: %v", err)
	}

	stream, err := driverMap.Watch(ctx)
	if err != nil {
		log.Printf("[Devices] Failed to watch device map: %v", err)
		return
	}

	for {
		entry, err := stream.Next()
		if err != nil {
			log.Printf("[Devices] Error in device stream: %v", err)
			continue
		}
		log.Printf("[Devices] Device event: %s", entry.String())
		dev := &Device{
			ID:     entry.Key,
			Driver: &FakeDriver{ID: entry.Key},
		}
		start(entry.Key, dev)
	}
}
