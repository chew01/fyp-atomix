package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type MembersResponse struct {
	Members       []string  `json:"members"`
	Count         int       `json:"count"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

func (s *Server) GetMembersHandler(w http.ResponseWriter, r *http.Request) {
	membershipManager := s.membershipManager

	// Collect all members
	var members []string
	for member := range membershipManager.Active {
		members = append(members, member)
	}

	resp := MembersResponse{
		Members:       members,
		Count:         len(members),
		LastUpdatedAt: time.Now(), // in a real setup, you might get this from Watch()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
