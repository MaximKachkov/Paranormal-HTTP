package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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

// То, что сохраняется в storage/entities/{id}.json.
type Dossier struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ThreatLevel     int      `json:"threat_level"`
	Vulnerabilities []string `json:"vulnerabilities"`
	EvidenceURLs    []string `json:"evidence_urls"`
}

// Информация о файле, который сохранить не удалось.
type FailedEvidence struct {
	FileName string `json:"file_name"`
	Reason   string `json:"reason"`
}

// Ответ POST /api/v1/entities.
type Response struct {
	ID                string           `json:"id"`
	Status            string           `json:"status"`
	SavedEvidenceURLs []string         `json:"saved_evidence_urls"`
	FailedEvidence    []FailedEvidence `json:"failed_evidence,omitempty"`
}

var (
	ErrFileEmpty       = errors.New("file_empty")
	ErrFileTooLarge    = errors.New("file_too_large")
	ErrInvalidMimeType = errors.New("invalid_mime_type")
)

var allowedMimeTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

func main() {
	logger := slog.New(
		slog.NewTextHandler(os.Stdout, nil),
	)

	var cfg Config

	flag.StringVar(
		&cfg.Address,
		"addr",
		":8080",
		"server address",
	)

	flag.StringVar(
		&cfg.Storage,
		"storage",
		"./storage",
		"storage directory",
	)

	flag.Parse()

	// Создаём директории.
	entityPath := filepath.Join(
		cfg.Storage,
		"entities",
	)

	evidencePath := filepath.Join(
		cfg.Storage,
		"evidence",
	)

	if err := os.MkdirAll(entityPath, 0755); err != nil {
		logger.Error(
			"failed to create entities directory",
			slog.Any("err", err),
		)
		return
	}

	if err := os.MkdirAll(evidencePath, 0755); err != nil {
		logger.Error(
			"failed to create evidence directory",
			slog.Any("err", err),
		)
		return
	}

	server := &Server{
		Logger:  logger,
		Storage: cfg.Storage,
	}

	mux := http.NewServeMux()

	mux.HandleFunc(
		"POST /api/v1/entities",
		server.PostEntity,
	)

	mux.HandleFunc(
		"GET /api/v1/entities/{id}",
		server.GetEntity,
	)

	mux.HandleFunc(
		"GET /api/v1/evidence/{filename}",
		server.GetEvidence,
	)

	srv := &http.Server{
		Addr:              cfg.Address,
		Handler:           server.RecoveryMiddleware(mux),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Запускаем сервер отдельно,
	// чтобы main мог одновременно ждать сигнал остановки.
	go func() {
		logger.Info(
			"application started",
			slog.String("addr", cfg.Address),
			slog.String("storage", cfg.Storage),
		)

		err := srv.ListenAndServe()

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error(
				"server error",
				slog.Any("err", err),
			)
		}
	}()

	// Блокируем main до Ctrl+C / SIGTERM.
	<-ctx.Done()

	logger.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(
			"graceful shutdown failed",
			slog.Any("err", err),
		)
		return
	}

	logger.Info("server stopped")
}

// RecoveryMiddleware не позволяет panic уронить весь сервер.
func (s *Server) RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.Logger.Error(
					"panic recovered",
					slog.Any("panic", recovered),
				)

				http.Error(
					w,
					"internal server error",
					http.StatusInternalServerError,
				)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// GET /api/v1/entities/{id}
func (s *Server) GetEntity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := uuid.Parse(id); err != nil {
		http.Error(
			w,
			"entity not found",
			http.StatusNotFound,
		)
		return
	}

	path := filepath.Join(
		s.Storage,
		"entities",
		id+".json",
	)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(
				w,
				"entity not found",
				http.StatusNotFound,
			)
			return
		}

		s.Logger.Error(
			"failed to read entity",
			slog.String("path", path),
			slog.Any("err", err),
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if _, err := w.Write(data); err != nil {
		s.Logger.Error(
			"failed to write entity response",
			slog.String("path", path),
			slog.Any("err", err),
		)
	}
}

// GET /api/v1/evidence/{filename}
func (s *Server) GetEvidence(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")

	base := filepath.Base(filename)

	// Защита от path traversal.
	if filename != base ||
		base == "" ||
		base == "." ||
		base == ".." {

		http.Error(
			w,
			"file not found",
			http.StatusNotFound,
		)
		return
	}

	path := filepath.Join(
		s.Storage,
		"evidence",
		filename,
	)

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(
				w,
				"file not found",
				http.StatusNotFound,
			)
			return
		}

		s.Logger.Error(
			"failed to open evidence",
			slog.String("path", path),
			slog.Any("err", err),
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		s.Logger.Error(
			"failed to stat evidence",
			slog.String("path", path),
			slog.Any("err", err),
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	// Не разрешаем отдавать директории.
	if info.IsDir() {
		http.Error(
			w,
			"file not found",
			http.StatusNotFound,
		)
		return
	}

	// ServeContent автоматически поддерживает Range.
	http.ServeContent(
		w,
		r,
		filename,
		info.ModTime(),
		file,
	)
}

// POST /api/v1/entities
func (s *Server) PostEntity(w http.ResponseWriter, r *http.Request) {
	// Максимальный размер всего HTTP body — 20 MB.
	r.Body = http.MaxBytesReader(
		w,
		r.Body,
		20<<20,
	)
	defer r.Body.Close()

	// 2 MB — сколько multipart может держать
	// в памяти. Это НЕ общий лимит запроса.
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		var maxErr *http.MaxBytesError

		if errors.As(err, &maxErr) {
			http.Error(
				w,
				"request too large",
				http.StatusRequestEntityTooLarge,
			)
			return
		}

		s.Logger.Warn(
			"failed to parse multipart",
			slog.Any("err", err),
		)

		http.Error(
			w,
			"invalid multipart form",
			http.StatusBadRequest,
		)
		return
	}

	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	// -------------------------------
	// DOSSIER
	// -------------------------------

	dossierJSON := r.FormValue("dossier")

	if dossierJSON == "" {
		http.Error(
			w,
			"dossier is required",
			http.StatusBadRequest,
		)
		return
	}

	// dossier JSON максимум 1 MB.
	if len(dossierJSON) > 1<<20 {
		writeJSON(
			w,
			http.StatusBadRequest,
			map[string]string{
				"error": "dossier json size exceeds 1MB limit",
			},
		)
		return
	}

	var dossierInput DossierInput

	decoder := json.NewDecoder(
		strings.NewReader(dossierJSON),
	)

	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&dossierInput); err != nil {
		s.Logger.Warn(
			"invalid dossier",
			slog.Any("err", err),
		)

		http.Error(
			w,
			"invalid dossier",
			http.StatusBadRequest,
		)
		return
	}

	// Проверяем, что после первого JSON-объекта
	// больше ничего нет.
	var extra any

	err := decoder.Decode(&extra)

	if !errors.Is(err, io.EOF) {
		http.Error(
			w,
			"invalid dossier",
			http.StatusBadRequest,
		)
		return
	}

	// Все поля обязательные.
	if strings.TrimSpace(dossierInput.Name) == "" {
		http.Error(
			w,
			"name is required",
			http.StatusBadRequest,
		)
		return
	}

	if strings.TrimSpace(dossierInput.Description) == "" {
		http.Error(
			w,
			"description is required",
			http.StatusBadRequest,
		)
		return
	}

	if dossierInput.ThreatLevel < 1 ||
		dossierInput.ThreatLevel > 10 {

		http.Error(
			w,
			"invalid threat_level",
			http.StatusBadRequest,
		)
		return
	}

	if dossierInput.Vulnerabilities == nil {
		http.Error(
			w,
			"vulnerabilities is required",
			http.StatusBadRequest,
		)
		return
	}

	// -------------------------------
	// EVIDENCE
	// -------------------------------

	evidences := r.MultipartForm.File["evidence"]

	if len(evidences) < 1 || len(evidences) > 10 {
		http.Error(
			w,
			"evidence count must be between 1 and 10",
			http.StatusBadRequest,
		)
		return
	}

	uniqueID := uuid.NewString()

	dossier := Dossier{
		ID:              uniqueID,
		Name:            dossierInput.Name,
		Description:     dossierInput.Description,
		ThreatLevel:     dossierInput.ThreatLevel,
		Vulnerabilities: dossierInput.Vulnerabilities,
		EvidenceURLs:    make([]string, 0, len(evidences)),
	}

	response := Response{
		ID:                uniqueID,
		SavedEvidenceURLs: make([]string, 0, len(evidences)),
	}

	// Здесь храним именно пути на диске,
	// чтобы удалить их при rollback.
	savedPaths := make([]string, 0, len(evidences))

	for _, evidence := range evidences {
		fullURL, filePath, err := s.ProcessSingleFile(evidence)

		if err != nil {
			// Ошибка конкретного пользовательского файла.
			if errors.Is(err, ErrFileEmpty) ||
				errors.Is(err, ErrFileTooLarge) ||
				errors.Is(err, ErrInvalidMimeType) {

				response.FailedEvidence = append(
					response.FailedEvidence,
					FailedEvidence{
						FileName: evidence.Filename,
						Reason:   err.Error(),
					},
				)

				continue
			}

			// Инфраструктурная ошибка.
			s.rollbackFiles(savedPaths)

			s.Logger.Error(
				"internal error while processing evidence",
				slog.String(
					"filename",
					evidence.Filename,
				),
				slog.Any("err", err),
			)

			http.Error(
				w,
				"internal server error",
				http.StatusInternalServerError,
			)
			return
		}

		response.SavedEvidenceURLs = append(
			response.SavedEvidenceURLs,
			fullURL,
		)

		dossier.EvidenceURLs = append(
			dossier.EvidenceURLs,
			fullURL,
		)

		savedPaths = append(
			savedPaths,
			filePath,
		)
	}

	// Ни одного evidence сохранить не получилось.
	if len(response.SavedEvidenceURLs) == 0 {
		http.Error(
			w,
			"no evidence saved",
			http.StatusBadRequest,
		)
		return
	}

	// -------------------------------
	// СОХРАНЕНИЕ DOSSIER
	// -------------------------------

	if err := s.AddDossier(dossier); err != nil {
		s.rollbackFiles(savedPaths)

		s.Logger.Error(
			"failed to add dossier",
			slog.String("id", uniqueID),
			slog.Any("err", err),
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	// -------------------------------
	// RESPONSE
	// -------------------------------

	statusCode := http.StatusCreated
	response.Status = "success"

	if len(response.FailedEvidence) > 0 {
		statusCode = http.StatusMultiStatus
		response.Status = "partial_success"
	}

	data, err := json.Marshal(response)
	if err != nil {
		s.Logger.Error(
			"failed to marshal response",
			slog.Any("err", err),
		)

		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	if statusCode == http.StatusCreated {
		w.Header().Set(
			"Location",
			"/api/v1/entities/"+uniqueID,
		)
	}

	w.WriteHeader(statusCode)

	if _, err := w.Write(data); err != nil {
		s.Logger.Error(
			"failed to write response",
			slog.Any("err", err),
		)
	}
}

// ProcessSingleFile проверяет и сохраняет один evidence.
func (s *Server) ProcessSingleFile(
	h *multipart.FileHeader,
) (string, string, error) {

	filename := filepath.Base(h.Filename)

	// 3 MB.
	if h.Size > 3<<20 {
		s.Logger.Warn(
			"file too large",
			slog.String("filename", filename),
			slog.Int64("size", h.Size),
		)

		return "", "", ErrFileTooLarge
	}

	if h.Size == 0 {
		s.Logger.Warn(
			"file is empty",
			slog.String("filename", filename),
		)

		return "", "", ErrFileEmpty
	}

	file, err := h.Open()
	if err != nil {
		s.Logger.Error(
			"failed to open uploaded file",
			slog.String("filename", filename),
			slog.Any("err", err),
		)

		return "", "", err
	}
	defer file.Close()

	// DetectContentType смотрит максимум первые 512 байт.
	buffer := make([]byte, 512)

	n, err := file.Read(buffer)

	if err != nil && !errors.Is(err, io.EOF) {
		s.Logger.Error(
			"failed to read uploaded file",
			slog.String("filename", filename),
			slog.Any("err", err),
		)

		return "", "", err
	}

	if n == 0 {
		return "", "", ErrFileEmpty
	}

	mimeType := http.DetectContentType(
		buffer[:n],
	)

	extension, ok := allowedMimeTypes[mimeType]
	if !ok {
		s.Logger.Warn(
			"invalid mime type",
			slog.String("filename", filename),
			slog.String("mime_type", mimeType),
		)

		return "", "", ErrInvalidMimeType
	}

	// После чтения первых 512 байт возвращаемся
	// в начало файла перед io.Copy.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		s.Logger.Error(
			"failed to seek uploaded file",
			slog.String("filename", filename),
			slog.Any("err", err),
		)

		return "", "", err
	}

	storedFilename := uuid.NewString() + extension

	fullPath := filepath.Join(
		s.Storage,
		"evidence",
		storedFilename,
	)

	dst, err := os.Create(fullPath)
	if err != nil {
		s.Logger.Error(
			"failed to create evidence file",
			slog.String("path", fullPath),
			slog.Any("err", err),
		)

		return "", "", err
	}

	// Не defer, потому что нам важно увидеть ошибку Close.
	_, copyErr := io.Copy(dst, file)

	closeErr := dst.Close()

	if copyErr != nil {
		_ = os.Remove(fullPath)

		s.Logger.Error(
			"failed to copy evidence",
			slog.String("path", fullPath),
			slog.Any("err", copyErr),
		)

		return "", "", copyErr
	}

	if closeErr != nil {
		_ = os.Remove(fullPath)

		s.Logger.Error(
			"failed to close evidence file",
			slog.String("path", fullPath),
			slog.Any("err", closeErr),
		)

		return "", "", closeErr
	}

	// Это URL, поэтому filepath.Join здесь НЕ используем.
	fullURL := "/api/v1/evidence/" + storedFilename

	return fullURL, fullPath, nil
}

// AddDossier атомарно сохраняет dossier:
//
// id.json.tmp
//
//	↓
//
// id.json
func (s *Server) AddDossier(d Dossier) error {
	data, err := json.MarshalIndent(
		d,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	dir := filepath.Join(
		s.Storage,
		"entities",
	)

	tmpPath := filepath.Join(
		dir,
		d.ID+".json.tmp",
	)

	finalPath := filepath.Join(
		dir,
		d.ID+".json",
	)

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		s.Logger.Error(
			"failed to create temporary dossier",
			slog.String("path", tmpPath),
			slog.Any("err", err),
		)

		return err
	}

	// Если что-то упадёт до Rename —
	// временный файл будет удалён.
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()

		s.Logger.Error(
			"failed to write temporary dossier",
			slog.String("path", tmpPath),
			slog.Any("err", err),
		)

		return err
	}

	// Просим ОС сбросить данные на диск.
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()

		s.Logger.Error(
			"failed to sync temporary dossier",
			slog.String("path", tmpPath),
			slog.Any("err", err),
		)

		return err
	}

	// На Windows особенно важно закрыть файл
	// до os.Rename.
	if err := tmpFile.Close(); err != nil {
		s.Logger.Error(
			"failed to close temporary dossier",
			slog.String("path", tmpPath),
			slog.Any("err", err),
		)

		return err
	}

	if err := os.Rename(
		tmpPath,
		finalPath,
	); err != nil {

		s.Logger.Error(
			"failed to rename dossier",
			slog.String("from", tmpPath),
			slog.String("to", finalPath),
			slog.Any("err", err),
		)

		return err
	}

	return nil
}

// rollbackFiles удаляет уже сохранённые evidence,
// если дальше произошла инфраструктурная ошибка.
func (s *Server) rollbackFiles(paths []string) {
	for _, path := range paths {
		if err := os.Remove(path); err != nil &&
			!errors.Is(err, os.ErrNotExist) {

			s.Logger.Error(
				"rollback failed",
				slog.String("path", path),
				slog.Any("err", err),
			)
		}
	}
}

// Маленький helper для JSON-ответов.
func writeJSON(
	w http.ResponseWriter,
	statusCode int,
	value any,
) {
	data, err := json.Marshal(value)
	if err != nil {
		http.Error(
			w,
			"internal server error",
			http.StatusInternalServerError,
		)
		return
	}

	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(statusCode)

	_, _ = w.Write(data)
}

// Пока оставил, если хочешь быстро дебажить входящий dossier.
func debugDossier(raw string) {
	fmt.Printf("RAW DOSSIER: %q\n", raw)
}
