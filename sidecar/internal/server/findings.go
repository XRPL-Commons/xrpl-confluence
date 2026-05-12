package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/finding"
)

func (s *Server) findingsList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 100
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, api.ErrorResponse{
				Error: api.Error{Code: "bad_request", Message: "limit must be a positive integer"},
			})
			return
		}
		limit = n
	}

	if s.findingStore == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]\n"))
		return
	}

	opts := finding.ListOpts{
		Since: q.Get("since"),
		Kind:  q.Get("kind"),
		Limit: limit,
	}
	results := s.findingStore.List(opts)
	if results == nil {
		results = []api.Finding{}
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) findingsByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.findingStore == nil {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeFindingNotFound, Message: "finding not found", Field: "id"},
		})
		return
	}

	f, ok := s.findingStore.GetByID(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{
			Error: api.Error{Code: api.ErrCodeFindingNotFound, Message: "finding not found", Field: "id"},
		})
		return
	}

	writeJSON(w, http.StatusOK, f)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
