package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON serializes v as JSON and writes it with the given status code.
// Errors during marshal are swallowed after writing a 500; callers should
// ensure v is always serializable.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error envelope: {"error":"<msg>"}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
