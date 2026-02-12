package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
)

// ServiceInfo represents a service in the API response.
type ServiceInfo struct {
	Name              string      `json:"name"`
	Emoji             string      `json:"emoji"`
	Status            unit.Status `json:"status"`
	CurrentStep       string      `json:"current_step,omitempty"`
	DependsOn         []string    `json:"depends_on"`
	UnmetDependencies []string    `json:"unmet_dependencies,omitempty"`
}

// ListServicesResponse is the response for GET /api/services.
type ListServicesResponse struct {
	Services []ServiceInfo `json:"services"`
}

func serviceNamesToStrings(names []catalog.ServiceName) []string {
	result := make([]string, len(names))
	for i, n := range names {
		result[i] = string(n)
	}
	return result
}

func (s *Server) buildListServicesResponse() ListServicesResponse {
	var services []ServiceInfo

	_ = s.catalog.ForEach(func(svc catalog.ServiceBase) error {
		status, err := s.catalog.Status(svc.Name())
		if err != nil {
			return err
		}

		info := ServiceInfo{
			Name:        string(svc.Name()),
			Emoji:       svc.Emoji(),
			Status:      status,
			CurrentStep: svc.CurrentStep(),
			DependsOn:   serviceNamesToStrings(svc.DependsOn()),
		}

		// Include unmet dependencies for non-completed services.
		if status != unit.StatusComplete {
			unmet, _ := s.catalog.UnmetDependencies(svc.Name())
			info.UnmetDependencies = unmet
		}

		services = append(services, info)
		return nil
	})

	return ListServicesResponse{Services: services}
}

func (s *Server) handleListServices(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, s.buildListServicesResponse())
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, ok := s.catalog.Get(catalog.ServiceName(name))
	if !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}

	status, err := s.catalog.Status(catalog.ServiceName(name))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to get service status: "+err.Error())
		return
	}

	info := ServiceInfo{
		Name:        string(svc.Name()),
		Emoji:       svc.Emoji(),
		Status:      status,
		CurrentStep: svc.CurrentStep(),
		DependsOn:   serviceNamesToStrings(svc.DependsOn()),
	}

	// Include unmet dependencies for non-completed services.
	if status != unit.StatusComplete {
		unmet, _ := s.catalog.UnmetDependencies(svc.Name())
		info.UnmetDependencies = unmet
	}

	s.writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleStartService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.catalog.Get(catalog.ServiceName(name)); !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err := s.catalog.StartService(r.Context(), catalog.ServiceName(name), s.logger); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to start service: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleRestartService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.catalog.Get(catalog.ServiceName(name)); !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err := s.catalog.RestartService(r.Context(), catalog.ServiceName(name), s.logger); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to restart service: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (s *Server) handleStopService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := s.catalog.Get(catalog.ServiceName(name)); !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err := s.catalog.StopService(r.Context(), catalog.ServiceName(name)); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to stop service: "+err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := s.catalog.Subscribe()
	defer s.catalog.Unsubscribe(sub)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastData []byte

	sendState := func() {
		data, err := json.Marshal(s.buildListServicesResponse())
		if err != nil {
			return
		}
		if bytes.Equal(data, lastData) {
			return
		}
		lastData = data
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Send initial state immediately.
	sendState()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-sub:
			sendState()
		case <-ticker.C:
			sendState()
		}
	}
}
