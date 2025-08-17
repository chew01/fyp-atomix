package main

import (
	"context"
	"fmt"
	"log"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

func main() {
	ctx := context.Background()

	testMap, err := atomix.Map[string, string]("test-map").Codec(generic.Scalar[string]()).Get(ctx)
	if err != nil {
		log.Fatalf("Failed to build Map primitive: %v", err)
	}

	fmt.Println("Putting key1 -> value1")
	if _, err := testMap.Put(ctx, "key1", "value1"); err != nil {
		log.Fatalf("Put failed: %v", err)
	}

	// 4. Get the value
	value, err := testMap.Get(ctx, "key1")
	if err != nil {
		log.Fatalf("Get failed: %v", err)
	}
	fmt.Printf("Got key1 -> %s\n", value)

	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key-%d", i)
		val := fmt.Sprintf("value-%d", i)
		_, err := testMap.Put(ctx, key, val)
		if err != nil {
			log.Fatalf("Put %s failed: %v", key, err)
		}
		v, _ := testMap.Get(ctx, key)
		fmt.Printf("%s -> %s\n", key, v)
	}

	fmt.Println("Map primitive test completed successfully")
}
