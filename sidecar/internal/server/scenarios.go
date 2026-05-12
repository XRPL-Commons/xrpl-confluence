package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/scenario"
)

type scenarioListItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

type scenarioListResponse struct {
	Scenarios []scenarioListItem `json:"scenarios"`
}

type validateResponse struct {
	OK     bool        `json:"ok"`
	Errors []api.Error `json:"errors"`
}

func (s *Server) scenariosList(w http.ResponseWriter, r *http.Request) {
	items := []scenarioListItem{}

	if s.scenariosDir != "" {
		entries, err := os.ReadDir(s.scenariosDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if !strings.HasSuffix(e.Name(), ".yaml") {
					continue
				}
				absPath := filepath.Join(s.scenariosDir, e.Name())
				sc, err := scenario.Load(absPath)
				if err != nil {
					continue
				}
				relPath, err := filepath.Rel(s.scenariosDir, absPath)
				if err != nil {
					relPath = e.Name()
				}
				items = append(items, scenarioListItem{
					Name:        sc.Metadata.Name,
					Description: sc.Metadata.Description,
					Path:        relPath,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, scenarioListResponse{Scenarios: items})
}

func (s *Server) scenariosValidate(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeUnsupportedMediaType, Message: "Content-Type must be application/json"},
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var payload api.Scenario
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeBadRequest, Message: err.Error()},
		})
		return
	}

	errs := scenario.Validate(&payload)
	if errs == nil {
		errs = []api.Error{}
	}
	writeJSON(w, http.StatusOK, validateResponse{OK: len(errs) == 0, Errors: errs})
}
