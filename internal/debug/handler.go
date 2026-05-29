package debug

import (
	"encoding/json"
	"net/http"
)

// StatsHandler returns an HTTP handler that exposes the debug tracer stats as JSON.
func StatsHandler(t *Tracer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(t.Stats())
	}
}
