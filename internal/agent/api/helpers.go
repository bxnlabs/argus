package api

import (
	"encoding/json"
	"log"
	"net/http"
)

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("json encode: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	if status >= 500 {
		log.Printf("HTTP %d: %s", status, message)
	}
	respondJSON(w, status, map[string]string{"error": message})
}

const maxBodySize = 1 << 20 // 1 MB

func parseBody(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
