package pb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type ServiceReading struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	HostIP    string    `json:"host_ip"`
	HostName  string    `json:"host_name"`
	URL       string    `json:"url,omitempty"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	LatencyMS int       `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
}
type ServiceSnapshot struct {
	CheckedAt time.Time        `json:"checked_at"`
	Services  []ServiceReading `json:"services"`
	Stale     bool             `json:"stale"`
}

func readServiceSnapshot(filename string, now time.Time) (ServiceSnapshot, error) {
	var value ServiceSnapshot
	file, err := os.Open(filename)
	if err != nil {
		return value, err
	}
	defer file.Close()
	if err := json.NewDecoder(io.LimitReader(file, 65536)).Decode(&value); err != nil {
		return value, err
	}
	if value.CheckedAt.IsZero() || len(value.Services) > 64 {
		return value, fmt.Errorf("invalid service snapshot")
	}
	value.Stale = now.Sub(value.CheckedAt) > 90*time.Second || value.CheckedAt.After(now.Add(10*time.Second))
	for i := range value.Services {
		if value.Stale || value.Services[i].CheckedAt.IsZero() || now.Sub(value.Services[i].CheckedAt) > 90*time.Second {
			value.Services[i].Status = "stale"
			value.Services[i].Detail = "No fresh result from Pi collector"
		}
	}
	return value, nil
}

func HandlerServices(e *core.RequestEvent) error {
	e.Response.Header().Set("Cache-Control", "no-store")
	value, err := readServiceSnapshot(os.Getenv("UPSNAP_SERVICES_FILE"), time.Now())
	if err != nil {
		return apis.NewApiError(http.StatusServiceUnavailable, "Service checks are unavailable.", nil)
	}
	visible := make([]ServiceReading, 0, len(value.Services))
	for _, service := range value.Services {
		device, err := e.App.FindFirstRecordByData("devices", "ip", service.HostIP)
		if err != nil {
			continue
		}
		if !e.HasSuperuserAuth() {
			if _, err := e.App.FindFirstRecordByFilter("permissions", "user.id = {:user} && read.id ?= {:device}",
				dbx.Params{"user": e.Auth.Id, "device": device.Id}); err != nil {
				continue
			}
		}
		service.HostName = device.GetString("name")
		if service.Status == "down" && device.GetString("status") == "offline" {
			service.Status = "offline"
			service.Detail += "; host SSH unavailable"
		}
		visible = append(visible, service)
	}
	value.Services = visible
	return e.JSON(http.StatusOK, value)
}
