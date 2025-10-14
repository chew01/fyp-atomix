package api

import "net/http"

func (s *Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}
