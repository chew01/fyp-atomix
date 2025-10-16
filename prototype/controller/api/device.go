package api

import (
	"encoding/json"
	"net/http"
	"prototype/controller/device"
)

type AddDeviceRequest struct {
	DeviceID string `json:"device_id"`
}

func (s *Server) AddDeviceHandler(w http.ResponseWriter, r *http.Request) {
	// Parse JSON body
	var req AddDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}

	// Create new Device
	newDev := device.NewDevice(req.DeviceID, &device.FakeDriver{ID: req.DeviceID})
	newDev.ApplyConfig(s.ctx, make(map[string]string))

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Device added successfully"))
}
