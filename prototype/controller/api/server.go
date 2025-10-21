package api

import (
	"context"
	"log"
	"net/http"
	"prototype/controller/membership"
)

type Server struct {
	ctx               context.Context
	membershipManager *membership.MembershipManager
}

func StartServer(ctx context.Context, membershipManager *membership.MembershipManager, port string) {
	s := &Server{ctx: ctx, membershipManager: membershipManager}
	r := s.NewRouter()

	log.Printf("Starting HTTP server on port %s", port)
	if err := http.ListenAndServe(port, r); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
