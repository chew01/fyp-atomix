package api

import (
	"github.com/gorilla/mux"
)

func (s *Server) NewRouter() *mux.Router {
	r := mux.NewRouter()

	// Health check
	r.HandleFunc("/health", s.HealthHandler).Methods("GET")
	// Membership
	r.HandleFunc("/members", s.GetMembersHandler).Methods("GET")

	return r
}
