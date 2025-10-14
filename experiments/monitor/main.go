package main

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(fmt.Sprintf("failed to get in-cluster config"))
	}
	_, err = dynamic.NewForConfig(config)

}
