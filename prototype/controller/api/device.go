package api

import (
	"encoding/json"
	"log"
	"net/http"
	"prototype/controller/device"
	"time"

	"github.com/atomix/go-sdk/pkg/atomix"
	"github.com/atomix/go-sdk/pkg/generic"
)

type AddDeviceRequest struct {
	DeviceID string `json:"device_id"`
}

type DeviceResponse struct {
	Devices       map[string]string `json:"devices"`
	Count         int               `json:"count"`
	LastUpdatedAt time.Time         `json:"last_updated_at"`
}

func (s *Server) ListDevicesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := s.ctx

	driverMap, err := atomix.Map[string, string]("device").
		Codec(generic.Scalar[string]()).
		Get(ctx)
	if err != nil {
		log.Fatalf("[Devices] Failed to get device map: %v", err)
	}

	var devices map[string]string = make(map[string]string)
	stream, err := driverMap.List(ctx)
	if err != nil {
		http.Error(w, "Failed to read device list", http.StatusInternalServerError)
		return
	}
	for {
		elem, err := stream.Next()
		if err != nil {
			break
		}
		devices[elem.Key] = elem.Value
	}

	resp := DeviceResponse{
		Devices:       devices,
		Count:         len(devices),
		LastUpdatedAt: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

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
