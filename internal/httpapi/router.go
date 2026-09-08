package httpapi

import (
	"net/http"
	"paranormal-http/internal/middleware"
)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/entities", s.PostEntity)
	mux.HandleFunc("GET /api/v1/entities/{id}", s.GetEntity)
	mux.HandleFunc("GET /api/v1/evidence/{filename}", s.GetEvidence)
	return middleware.Recovery(s.Logger, mux)
}
