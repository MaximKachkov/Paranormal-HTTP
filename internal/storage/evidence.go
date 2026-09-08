package storage

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

var (
	ErrFileEmpty       = errors.New("file_empty")
	ErrFileTooLarge    = errors.New("file_too_large")
	ErrInvalidMimeType = errors.New("invalid_mime_type")
)

var allowedMimeTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
}

func (s *FileStore) ProcessSingleFile(h *multipart.FileHeader) (string, string, int64, error) {
	originalFilename := filepath.Base(h.Filename)
	if h.Size == 0 {
		s.Logger.Warn("empty evidence", slog.String("filename", originalFilename))
		return "", "", 0, ErrFileEmpty
	}

	if h.Size > 3<<20 {
		s.Logger.Warn("evidence too large", slog.String("filename", originalFilename), slog.Int64("size", h.Size))
		return "", "", 0, ErrFileTooLarge
	}

	file, err := h.Open()
	if err != nil {
		return "", "", 0, fmt.Errorf("open evidence: %w", err)
	}

	defer file.Close()
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", 0, fmt.Errorf("read evidence header: %w", err)
	}

	if n == 0 {
		return "", "", 0, ErrFileEmpty
	}

	mimeType := http.DetectContentType(buffer[:n])
	extension, ok := allowedMimeTypes[mimeType]
	if !ok {
		s.Logger.Warn(
			"invalid evidence MIME type", slog.String("filename", originalFilename), slog.String("mime_type", mimeType))
		return "", "", 0, ErrInvalidMimeType
	}

	// Возвращаемся к началу файла.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", 0, fmt.Errorf("seek evidence: %w", err)
	}

	storedFilename := uuid.NewString() + extension
	fullPath := filepath.Join(s.Root, "evidence", storedFilename)
	dst, err := os.Create(fullPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("create evidence file: %w", err)
	}

	written, err := io.Copy(dst, file)
	if err != nil {
		_ = dst.Close()
		_ = os.Remove(fullPath)
		return "", "", 0, fmt.Errorf("save evidence: %w", err)
	}

	// Ошибка Close тоже важна.
	if err := dst.Close(); err != nil {
		_ = os.Remove(fullPath)
		return "", "", 0, fmt.Errorf("close evidence file: %w", err)
	}

	fullURL := "/api/v1/evidence/" + storedFilename
	return fullURL, fullPath, written, nil
}

func RollbackFiles(paths []string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
