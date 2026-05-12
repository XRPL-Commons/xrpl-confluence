package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/XRPL-Commons/xrpl-confluence/sidecar/internal/api"
)

// HealthzResponse is the body for GET /v1/healthz.
type HealthzResponse struct {
	OK              bool   `json:"ok"`
	APIVersion      string `json:"api_version"`
	UptimeS         int    `json:"uptime_s"`
	Scenario        string `json:"scenario"`
	BudgetRemainingS *int  `json:"budget_remaining_s,omitempty"`
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	body := HealthzResponse{
		OK:         true,
		APIVersion: api.Version,
		UptimeS:    int(time.Since(s.startedAt).Seconds()),
		Scenario:   s.scenario,
	}

	if !s.budgetDeadline.IsZero() {
		rem := int(time.Until(s.budgetDeadline).Seconds())
		if rem < 0 {
			rem = 0
		}
		body.BudgetRemainingS = &rem
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}
