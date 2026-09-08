package httpapi

import (
	"encoding/json"
	"net/http"
)

type FailedEvidence struct {
	FileName string `json:"file_name"`
	Reason   string `json:"reason"`
}

type Response struct {
	ID                string           `json:"id"`
	Status            string           `json:"status"`
	SavedEvidenceURLs []string         `json:"saved_evidence_urls"`
	FailedEvidence    []FailedEvidence `json:"failed_evidence,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
