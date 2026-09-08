package httpapi

import (
	"log/slog"

	"paranormal-http/internal/service"
	"paranormal-http/internal/storage"
)

type Server struct {
	Logger   *slog.Logger
	Store    *storage.FileStore
	Entities *service.Entity
}

func New(logger *slog.Logger, store *storage.FileStore) *Server {
	return &Server{Logger: logger, Store: store, Entities: service.NewEntity(store)}
}
