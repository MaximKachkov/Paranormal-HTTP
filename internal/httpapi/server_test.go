package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"paranormal-http/internal/model"
	"paranormal-http/internal/storage"
)

// ------------------------------
// HELPERS
// ------------------------------

func newTestServer(t *testing.T) *Server {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := storage.New(t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	return New(logger, store)
}

// Настоящие минимальные JPEG-байты.
// http.DetectContentType определяет их как image/jpeg.
func fakeJPEG() []byte {
	return []byte{
		0xFF, 0xD8, 0xFF, 0xE0,
		0x00, 0x10,
		0x4A, 0x46, 0x49, 0x46,
		0x00, 0x01,
		0x01, 0x01,
		0x00, 0x48,
		0x00, 0x48,
		0x00, 0x00,
		0xFF, 0xD9,
	}
}

func validDossierJSON() string {
	return `{
		"name":"Ghost",
		"description":"Scary ghost",
		"threat_level":7,
		"vulnerabilities":["salt","light"]
	}`
}

type uploadFile struct {
	FieldName string
	FileName  string
	Data      []byte
}

func makeMultipartRequest(
	t *testing.T,
	dossier *string,
	files []uploadFile,
) *http.Request {
	t.Helper()

	var body bytes.Buffer

	writer := multipart.NewWriter(&body)

	if dossier != nil {
		if err := writer.WriteField(
			"dossier",
			*dossier,
		); err != nil {
			t.Fatalf(
				"failed to write dossier field: %v",
				err,
			)
		}
	}

	for _, f := range files {
		part, err := writer.CreateFormFile(
			f.FieldName,
			f.FileName,
		)

		if err != nil {
			t.Fatalf(
				"failed to create multipart file: %v",
				err,
			)
		}

		if _, err := part.Write(f.Data); err != nil {
			t.Fatalf(
				"failed to write multipart file: %v",
				err,
			)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf(
			"failed to close multipart writer: %v",
			err,
		)
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/entities",
		&body,
	)

	req.Header.Set(
		"Content-Type",
		writer.FormDataContentType(),
	)

	return req
}

// ------------------------------
// POST TESTS
// ------------------------------

func TestPostEntitySuccess(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf(
			"expected status %d, got %d, body: %s",
			http.StatusCreated,
			rec.Code,
			rec.Body.String(),
		)
	}

	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf(
			"expected application/json, got %q",
			rec.Header().Get("Content-Type"),
		)
	}

	if rec.Header().Get("Location") == "" {
		t.Error("expected Location header")
	}

	var response Response

	if err := json.Unmarshal(
		rec.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatalf(
			"invalid response JSON: %v",
			err,
		)
	}

	if response.Status != "success" {
		t.Errorf(
			"expected status success, got %q",
			response.Status,
		)
	}

	if response.ID == "" {
		t.Error("expected non-empty ID")
	}

	if len(response.SavedEvidenceURLs) != 1 {
		t.Fatalf(
			"expected 1 saved evidence, got %d",
			len(response.SavedEvidenceURLs),
		)
	}

	entityPath := filepath.Join(
		server.Store.Root,
		"entities",
		response.ID+".json",
	)

	if _, err := os.Stat(entityPath); err != nil {
		t.Errorf(
			"expected dossier file to exist: %v",
			err,
		)
	}
}

func TestPostEntityInvalidJSON(t *testing.T) {
	server := newTestServer(t)

	dossier := `{"name":`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityMissingDossier(t *testing.T) {
	server := newTestServer(t)

	req := makeMultipartRequest(
		t,
		nil,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}

	if !strings.Contains(
		rec.Body.String(),
		"dossier is required",
	) {
		t.Errorf(
			"unexpected body: %s",
			rec.Body.String(),
		)
	}
}

func TestPostEntityUnknownJSONField(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":7,
		"vulnerabilities":["salt"],
		"unknown":"test"
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d, body: %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}

func TestPostEntityExtraJSON(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":7,
		"vulnerabilities":["salt"]
	} {"hello":"world"}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityMissingName(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"description":"Ghost",
		"threat_level":7,
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityMissingDescription(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"threat_level":7,
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityMissingVulnerabilities(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":7
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityThreatLevelZero(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":0,
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityThreatLevelEleven(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":11,
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityThreatLevelOne(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":1,
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf(
			"expected 201, got %d, body: %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}

func TestPostEntityThreatLevelTen(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":10,
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf(
			"expected 201, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityNoEvidence(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	req := makeMultipartRequest(
		t,
		&dossier,
		nil,
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityEmptyEvidence(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "empty.jpg",
				Data:      []byte{},
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityInvalidMimeType(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "fake.jpg",
				Data:      []byte("this is text, not jpeg"),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityFileTooLarge(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	largeFile := make(
		[]byte,
		3<<20+1,
	)

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "big.jpg",
				Data:      largeFile,
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityPartialSuccess(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
			{
				FieldName: "evidence",
				FileName:  "bad.txt",
				Data:      []byte("not an image"),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusMultiStatus {
		t.Fatalf(
			"expected 207, got %d, body: %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	var response Response

	if err := json.Unmarshal(
		rec.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatalf(
			"invalid JSON response: %v",
			err,
		)
	}

	if response.Status != "partial_success" {
		t.Errorf(
			"expected partial_success, got %q",
			response.Status,
		)
	}

	if len(response.SavedEvidenceURLs) != 1 {
		t.Errorf(
			"expected 1 successful file, got %d",
			len(response.SavedEvidenceURLs),
		)
	}

	if len(response.FailedEvidence) != 1 {
		t.Errorf(
			"expected 1 failed file, got %d",
			len(response.FailedEvidence),
		)
	}

	if response.FailedEvidence[0].Reason != "invalid_mime_type" {
		t.Errorf(
			"expected invalid_mime_type, got %q",
			response.FailedEvidence[0].Reason,
		)
	}
}

func TestPostEntityMoreThanTenEvidence(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	files := make(
		[]uploadFile,
		0,
		11,
	)

	for i := 0; i < 11; i++ {
		files = append(
			files,
			uploadFile{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		)
	}

	req := makeMultipartRequest(
		t,
		&dossier,
		files,
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

func TestPostEntityWrongThreatType(t *testing.T) {
	server := newTestServer(t)

	dossier := `{
		"name":"Ghost",
		"description":"Ghost",
		"threat_level":"seven",
		"vulnerabilities":["salt"]
	}`

	req := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	rec := httptest.NewRecorder()

	server.PostEntity(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected 400, got %d",
			rec.Code,
		)
	}
}

// ------------------------------
// GET ENTITY TESTS
// ------------------------------

func TestGetEntitySuccess(t *testing.T) {
	server := newTestServer(t)

	dossier := validDossierJSON()

	postReq := makeMultipartRequest(
		t,
		&dossier,
		[]uploadFile{
			{
				FieldName: "evidence",
				FileName:  "cat.jpg",
				Data:      fakeJPEG(),
			},
		},
	)

	postRec := httptest.NewRecorder()

	server.PostEntity(
		postRec,
		postReq,
	)

	if postRec.Code != http.StatusCreated {
		t.Fatalf(
			"setup POST failed: %d %s",
			postRec.Code,
			postRec.Body.String(),
		)
	}

	var response Response

	if err := json.Unmarshal(
		postRec.Body.Bytes(),
		&response,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/entities/"+response.ID,
		nil,
	)

	req.SetPathValue(
		"id",
		response.ID,
	)

	rec := httptest.NewRecorder()

	server.GetEntity(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d",
			rec.Code,
		)
	}

	var entity model.Dossier

	if err := json.Unmarshal(
		rec.Body.Bytes(),
		&entity,
	); err != nil {
		t.Fatalf(
			"invalid entity JSON: %v",
			err,
		)
	}

	if entity.ID != response.ID {
		t.Errorf(
			"expected id %q, got %q",
			response.ID,
			entity.ID,
		)
	}
}

func TestGetEntityInvalidID(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/entities/hello",
		nil,
	)

	req.SetPathValue(
		"id",
		"hello",
	)

	rec := httptest.NewRecorder()

	server.GetEntity(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d",
			rec.Code,
		)
	}
}

func TestGetEntityNotFound(t *testing.T) {
	server := newTestServer(t)

	id := "11111111-1111-1111-1111-111111111111"

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/entities/"+id,
		nil,
	)

	req.SetPathValue(
		"id",
		id,
	)

	rec := httptest.NewRecorder()

	server.GetEntity(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d",
			rec.Code,
		)
	}
}

// ------------------------------
// GET EVIDENCE TESTS
// ------------------------------

func TestGetEvidenceSuccess(t *testing.T) {
	server := newTestServer(t)

	filename := "test.jpg"

	path := filepath.Join(
		server.Store.Root,
		"evidence",
		filename,
	)

	if err := os.WriteFile(
		path,
		fakeJPEG(),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/evidence/"+filename,
		nil,
	)

	req.SetPathValue(
		"filename",
		filename,
	)

	rec := httptest.NewRecorder()

	server.GetEvidence(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d",
			rec.Code,
		)
	}

	if !bytes.Equal(
		rec.Body.Bytes(),
		fakeJPEG(),
	) {
		t.Error(
			"returned evidence differs from stored evidence",
		)
	}
}

func TestGetEvidenceNotFound(t *testing.T) {
	server := newTestServer(t)

	filename := "missing.jpg"

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/evidence/"+filename,
		nil,
	)

	req.SetPathValue(
		"filename",
		filename,
	)

	rec := httptest.NewRecorder()

	server.GetEvidence(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d",
			rec.Code,
		)
	}
}

func TestGetEvidenceRange(t *testing.T) {
	server := newTestServer(t)

	filename := "range.jpg"

	data := make(
		[]byte,
		1000,
	)

	for i := range data {
		data[i] = byte(i % 255)
	}

	path := filepath.Join(
		server.Store.Root,
		"evidence",
		filename,
	)

	if err := os.WriteFile(
		path,
		data,
		0644,
	); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/evidence/"+filename,
		nil,
	)

	req.SetPathValue(
		"filename",
		filename,
	)

	req.Header.Set(
		"Range",
		"bytes=0-99",
	)

	rec := httptest.NewRecorder()

	server.GetEvidence(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf(
			"expected 206, got %d",
			rec.Code,
		)
	}

	if rec.Body.Len() != 100 {
		t.Fatalf(
			"expected 100 bytes, got %d",
			rec.Body.Len(),
		)
	}

	contentRange := rec.Header().Get(
		"Content-Range",
	)

	if !strings.HasPrefix(
		contentRange,
		"bytes 0-99/",
	) {
		t.Errorf(
			"unexpected Content-Range: %q",
			contentRange,
		)
	}
}

func TestGetEvidencePathTraversal(t *testing.T) {
	server := newTestServer(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/evidence/test",
		nil,
	)

	req.SetPathValue(
		"filename",
		"../entities/test.json",
	)

	rec := httptest.NewRecorder()

	server.GetEvidence(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404, got %d",
			rec.Code,
		)
	}
}
