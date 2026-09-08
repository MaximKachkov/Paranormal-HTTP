package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerRoutes(t *testing.T) {
	handler := newTestServer(t).Handler()
	dossier := validDossierJSON()
	req := makeMultipartRequest(t, &dossier, []uploadFile{
		{FieldName: "evidence", FileName: "ghost.jpg", Data: fakeJPEG()},
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST: got %d: %s", rec.Code, rec.Body.String())
	}
	var response Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, path := range append([]string{rec.Header().Get("Location")}, response.SavedEvidenceURLs...) {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("GET: got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
