package api

import (
	"net/http"

	"github.com/coder/coder/v2/scripts/cdev/catalog"
)

// ServiceInfo represents a service in the API response.
type ServiceInfo struct {
	Name      string   `json:"name"`
	Emoji     string   `json:"emoji"`
	Status    string   `json:"status"`
	DependsOn []string `json:"depends_on"`
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

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	var services []ServiceInfo

	_ = s.catalog.ForEach(func(svc catalog.ServiceBase) error {
		info := ServiceInfo{
			Name:      string(svc.Name()),
			Emoji:     svc.Emoji(),
			Status:    "running", // TODO: Track actual status in catalog.
			DependsOn: serviceNamesToStrings(svc.DependsOn()),
		}
		services = append(services, info)
		return nil
	})

	s.writeJSON(w, http.StatusOK, ListServicesResponse{Services: services})
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, ok := s.catalog.Get(catalog.ServiceName(name))
	if !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}

	info := ServiceInfo{
		Name:      string(svc.Name()),
		Emoji:     svc.Emoji(),
		Status:    "running", // TODO: Track actual status in catalog.
		DependsOn: serviceNamesToStrings(svc.DependsOn()),
	}

	s.writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleRestartService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, ok := s.catalog.Get(catalog.ServiceName(name))
	if !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}

	ctx := r.Context()

	// Stop then start the service.
	if err := svc.Stop(ctx); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to stop service: "+err.Error())
		return
	}

	if err := svc.Start(ctx, s.logger, s.catalog); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to start service: "+err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (s *Server) handleStopService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, ok := s.catalog.Get(catalog.ServiceName(name))
	if !ok {
		s.writeError(w, http.StatusNotFound, "service not found")
		return
	}

	if err := svc.Stop(r.Context()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "failed to stop service: "+err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
