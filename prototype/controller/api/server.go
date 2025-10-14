package api

import (
	"context"
	"log"
	"net/http"
)

type Server struct {
	ctx context.Context
}

func StartServer(ctx context.Context, port string) {
	s := &Server{ctx: ctx}
	r := s.NewRouter()

	log.Printf("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(port, r); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
