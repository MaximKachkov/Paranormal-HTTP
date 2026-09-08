package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) GetEvidence(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" || filename == "." || filename == ".." || strings.ContainsAny(filename, `/\`) || filepath.Base(filename) != filename {
		http.NotFound(w, r)
		return
	}

	file, err := s.Store.OpenEvidence(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}

		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Нельзя листить директории.
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// ServeContent автоматически
	// поддерживает Range requests.
	http.ServeContent(w, r, filename, info.ModTime(), file)
}
