package main

import (
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	Address string
	Storage string
}

type Server struct {
	Logger  *slog.Logger
	Storage string
}

// То, что клиент присылает в поле dossier.
type DossierInput struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ThreatLevel     int      `json:"threat_level"`
	Vulnerabilities []string `json:"vulnerabilities"`
}

// То, что мы сохраняем в storage/entities/{id}.json.
type Dossier struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	ThreatLevel     int       `json:"threat_level"`
	Vulnerabilities []string  `json:"vulnerabilities"`
	EvidenceURLs    []string  `json:"evidence_urls"`
}

// Информация о конкретном evidence, который сохранить не удалось.
type FailedEvidence struct {
	FileName string `json:"file_name"`
	Reason   string `json:"reason"`
}

// Ответ POST /api/v1/entities.
type Response struct {
	ID                uuid.UUID        `json:"id"`
	Status            string           `json:"status"`
	SavedEvidenceURLs []string         `json:"saved_evidence_urls"`
	FailedEvidence    []FailedEvidence `json:"failed_evidence,omitempty"`
}

// Причины отказа при обработке отдельных evidence-файлов.
const (
	ReasonInvalidMimeType = "invalid_mime_type"
	ReasonFileTooLarge    = "file_too_large"
	ReasonFileEmpty       = "file_empty"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var cfg Config
	mux := http.NewServeMux()
	flag.StringVar(&cfg.Address, "addr", ":8080", "port num")
	flag.StringVar(&cfg.Storage, "storage", "./storage", "storage dir")

	flag.Parse()

	srv := &http.Server{
		Addr:              cfg.Address,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	//Создание Директорий
	entityPath := filepath.Join(cfg.Storage, "entities")
	evidencePath := filepath.Join(cfg.Storage, "evidence")
	if err := os.MkdirAll(entityPath, 0755); err != nil {
		logger.Error("problem creating entity dir")
		return
	}
	if err := os.MkdirAll(evidencePath, 0755); err != nil {
		logger.Error("problem creating evidence dir")
		return
	}
	////////////////////
	server := &Server{
		Logger:  logger,
		Storage: cfg.Storage,
	}
	mux.HandleFunc("POST /api/v1/entities", server.PostEntity)
	mux.HandleFunc("GET /api/v1/entities/{id}", server.GetEntity)
	mux.HandleFunc("GET /api/v1/evidence/{filename}", server.GetEvidence)
	logger.Info(
		"application started",
		slog.String("addr", cfg.Address),
		slog.String("storage", cfg.Storage),
		slog.Time("started_at", time.Now()),
	)

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("Server wasnt able to launch", slog.String("err", err.Error()))
		return

	}

}

func (s *Server) GetEntity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	_, err := uuid.Parse(id)
	if err != nil {
		http.Error(w, "entity not found", http.StatusNotFound)
		return
	}

	path := filepath.Join(s.Storage, "entities", id+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		s.Logger.Warn("unable to read file", slog.String("path", path), slog.Any("err", err))

		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "", http.StatusNotFound)
			return
		} else {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if _, err = w.Write(data); err != nil {
		s.Logger.Error("unable to write file", slog.String("path", path), slog.Any("err", err))
		return
	}

}

func (s *Server) GetEvidence(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")

	base := filepath.Base(filename)

	if filename != base || base == "" || base == "." || base == ".." {
		s.Logger.Warn(
			"user wrote wrong path",
			slog.String("file_name", filename),
		)

		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	path := filepath.Join(
		s.Storage,
		"evidence",
		filename,
	)

	file, err := os.Open(path)
	if err != nil {
		s.Logger.Warn("wasn't able to open path", slog.String("path", path), slog.Any("err", err))
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		s.Logger.Error("wasnt able to get info ", slog.String("file_name", filename), slog.Any("err", err))
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		s.Logger.Error("User tried to do dir traversal", slog.String("file_name", filename))
		http.Error(w, "", http.StatusNotFound)
		return
	}

	http.ServeContent(w, r, filename, info.ModTime(), file)

}

func (s *Server) PostEntity(w http.ResponseWriter, r *http.Request) {

}
