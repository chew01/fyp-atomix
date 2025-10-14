package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

type MembersResponse struct {
	Members       []string  `json:"members"`
	Count         int       `json:"count"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

func (s *Server) GetMembersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := s.ctx

	membershipSet, err := atomix.Set[string]("membership").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		http.Error(w, "Failed to access membership set", http.StatusInternalServerError)
		log.Printf("[Membership] Failed to get set: %v", err)
		return
	}
	defer membershipSet.Close(ctx)

	// Collect all members
	var members []string
	stream, err := membershipSet.Elements(ctx)
	if err != nil {
		http.Error(w, "Failed to read membership elements", http.StatusInternalServerError)
		return
	}
	for {
		elem, err := stream.Next()
		if err != nil {
			break // end of stream
		}
		members = append(members, elem)
	}

	resp := MembersResponse{
		Members:       members,
		Count:         len(members),
		LastUpdatedAt: time.Now(), // in a real setup, you might get this from Watch()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
