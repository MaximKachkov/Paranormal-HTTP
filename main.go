package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrFileTooLarge    = errors.New("file too large")
	ErrInvalidFileType = errors.New("invalid file type")
)

var allowedMIMETypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type UploadResponse struct {
	Uploaded     []string `json:"uploaded"`
	URLs         []string `json:"urls"`
	Failed       []string `json:"failed"`
	TotalSuccess int      `json:"total_success"`
	TotalFailed  int      `json:"total_failed"`
}

func processSingleFile(fh *multipart.FileHeader) (string, error) {
	filename := filepath.Base(fh.Filename)

	// Проверяем имя файла.
	if filename == "" || filename == "." || filename == ".." {
		return "", fmt.Errorf("invalid filename: %s", filename)
	}

	// Максимальный размер одного файла — 1 MB.
	if fh.Size > 1<<20 {
		return "", ErrFileTooLarge
	}

	file, err := fh.Open()
	if err != nil {
		return "", fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	// Читаем первые 512 байт для определения MIME-типа.
	buffer := make([]byte, 512)

	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("error reading file: %w", err)
	}

	mimeType := http.DetectContentType(buffer[:n])

	if _, ok := allowedMIMETypes[mimeType]; !ok {
		return "", ErrInvalidFileType
	}

	// Возвращаемся в начало файла перед копированием.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("error seeking file: %w", err)
	}

	// Получаем расширение исходного файла.
	ext := strings.ToLower(filepath.Ext(filename))

	// Генерируем уникальное имя.
	newFilename := generateID() + ext

	// Путь для сохранения.
	path := filepath.Join("uploads", newFilename)

	// Создаём директорию uploads, если её нет.
	if err := os.MkdirAll("uploads", 0755); err != nil {
		return "", fmt.Errorf("error creating uploads directory: %w", err)
	}

	// Создаём новый файл.
	dst, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("error creating file: %w", err)
	}

	// Если копирование завершится ошибкой — удаляем созданный файл.
	_, err = io.Copy(dst, file)
	if err != nil {
		dst.Close()
		_ = os.Remove(path)

		return "", fmt.Errorf("error saving file: %w", err)
	}

	// Закрываем destination.
	if err := dst.Close(); err != nil {
		_ = os.Remove(path)

		return "", fmt.Errorf("error closing file: %w", err)
	}

	return path, nil
}

func UploadsHandler(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем весь HTTP body 10 MB.
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	defer r.Body.Close()

	// Разрешаем только POST.
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим multipart/form-data.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError

		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Получаем файлы из поля "files".
	files := r.MultipartForm.File["files"]

	if len(files) == 0 {
		http.Error(w, "no files uploaded", http.StatusBadRequest)
		return
	}

	response := UploadResponse{
		Uploaded: make([]string, 0),
		URLs:     make([]string, 0),
		Failed:   make([]string, 0),
	}

	// Здесь храним именно error,
	// потому что response.Failed содержит только строки для JSON.
	var fileErrors []error

	for _, fh := range files {
		filename, err := processSingleFile(fh)

		if err != nil {
			// Оборачиваем через %w, чтобы errors.Is мог определить
			// ErrFileTooLarge / ErrInvalidFileType.
			wrappedErr := fmt.Errorf("process %s: %w", fh.Filename, err)

			fileErrors = append(fileErrors, wrappedErr)
			response.Failed = append(response.Failed, wrappedErr.Error())

			continue
		}

		// В uploaded сохраняем оригинальное имя.
		response.Uploaded = append(response.Uploaded, fh.Filename)

		// Клиенту возвращаем URL.
		response.URLs = append(
			response.URLs,
			"/uploads/"+filepath.Base(filename),
		)
	}

	response.TotalSuccess = len(response.Uploaded)
	response.TotalFailed = len(response.Failed)

	w.Header().Set("Content-Type", "application/json")

	// Определяем HTTP status.
	if response.TotalSuccess == 0 {
		hasInternalError := false

		for _, err := range fileErrors {
			if !errors.Is(err, ErrFileTooLarge) &&
				!errors.Is(err, ErrInvalidFileType) {
				hasInternalError = true
				break
			}
		}

		if hasInternalError {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	} else {
		// Хотя бы один файл успешно загружен.
		w.WriteHeader(http.StatusCreated)
	}

	_ = json.NewEncoder(w).Encode(response)
}

// Функция main и все тесты будут скрыты от вас при проверке на сайте.
func main() {
	// Если вы захотите запустить сервер локально для проверки: RUN=1 go run main.go
	if os.Getenv("RUN") == "1" {
		os.MkdirAll("uploads", 0755)
		http.HandleFunc("/uploads", UploadsHandler)
		fmt.Println("Сервер запущен локально на http://localhost:8080/uploads")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка запуска сервера: %v\n", err)
			os.Exit(1)
		}
		return
	}

	os.MkdirAll("uploads", 0755)
	defer os.RemoveAll("uploads")

	if !test1() || !test2() || !test3() || !test4() || !test5() || !test6() || !test7() ||
		!test8() || !test9() || !test10() || !test11() || !test12() {
		os.Exit(1)
	}
	fmt.Println("Все тесты успешно пройдены!")
}

type fileDef struct {
	name    string
	content []byte
}

func createMultipartRequest(files []fileDef) (*http.Request, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, fd := range files {
		part, err := writer.CreateFormFile("files", fd.name)
		if err != nil {
			return nil, err
		}
		part.Write(fd.content)
	}
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/uploads", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

// Проверяем, что хэндлер возвращает статус 400, если форма отправлена без файлов
func test1() bool {
	req, _ := createMultipartRequest([]fileDef{})
	w := httptest.NewRecorder()

	UploadsHandler(w, req)

	if w.Code != http.StatusBadRequest {
		fmt.Fprintf(os.Stderr, "Тест 1: ожидался статус 400 при пустом списке файлов, получен %d\n", w.Code)
		return false
	}
	return true
}

// Проверяем логику частичного успеха, а также правильное использование Seek и форматирования ошибок
func test2() bool {
	validPNG := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("a"), 600)...)
	validJPEG := append([]byte("\xff\xd8\xff\xdb\x00\x43\x00"), bytes.Repeat([]byte("b"), 600)...)
	fakeJPG := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF")
	bigFile := make([]byte, 1024*1024+5) // Больше 1MB

	req, _ := createMultipartRequest([]fileDef{
		{name: "good2.png", content: validPNG},
		{name: "good2.jpg", content: validJPEG},
		{name: "virus2.jpg", content: fakeJPG},
		{name: "huge2.png", content: bigFile},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	if w.Code != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Тест 2: ожидался статус 201 (частичный успех), получен %d\n", w.Code)
		return false
	}

	var resp UploadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Тест 2: ошибка декодирования JSON: %v\n", err)
		return false
	}

	if resp.TotalSuccess != 2 || resp.TotalFailed != 2 {
		fmt.Fprintf(os.Stderr, "Тест 2: неверная статистика. Ожидалось: TotalSuccess=2, TotalFailed=2. Получено: %d и %d\n", resp.TotalSuccess, resp.TotalFailed)
		return false
	}

	if len(resp.Failed) != 2 {
		fmt.Fprintf(os.Stderr, "Тест 2: ожидалось 2 элемента в массиве Failed, получено %d\n", len(resp.Failed))
		return false
	}

	hasInvalidType := false
	hasTooLarge := false
	for _, f := range resp.Failed {
		if f == "process virus2.jpg: invalid file type" {
			hasInvalidType = true
		}
		if f == "process huge2.png: file too large" {
			hasTooLarge = true
		}
	}

	if !hasInvalidType || !hasTooLarge {
		fmt.Fprintf(os.Stderr, "Тест 2: массив Failed не содержит строго ожидаемых текстов ошибок. Вы оборачиваете ошибку через 'fmt.Errorf(\"process %%s: %%w\", name, err)'? Получено: %v\n", resp.Failed)
		return false
	}

	// Проверяем, что файлы действительно сохранены
	for _, u := range resp.URLs {
		fileName := strings.TrimPrefix(u, "/uploads/")
		savedContent, err := os.ReadFile(filepath.Join("uploads", fileName))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Тест 2: файл %s не найден в uploads/\n", fileName)
			return false
		}
		if !bytes.Equal(savedContent, validPNG) && !bytes.Equal(savedContent, validJPEG) {
			fmt.Fprintf(os.Stderr, "Тест 2: содержимое %s не совпадает с отправленным\n", fileName)
			return false
		}
	}

	return true
}

// Проверяем статус 422, когда все переданные файлы оказались невалидными
func test3() bool {
	fakeJPG := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF")
	badData := []byte("just some random text without signature")

	req, _ := createMultipartRequest([]fileDef{
		{name: "virus3.jpg", content: fakeJPG},
		{name: "random3.png", content: badData},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		fmt.Fprintf(os.Stderr, "Тест 3: ожидался статус 422 (все файлы битые), получен %d\n", w.Code)
		return false
	}

	var resp UploadResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Тест 3: ошибка декодирования JSON: %v\n", err)
		return false
	}

	if resp.TotalSuccess != 0 || resp.TotalFailed != 2 {
		fmt.Fprintf(os.Stderr, "Тест 3: ожидалось TotalSuccess=0 и TotalFailed=2, получено %d и %d\n", resp.TotalSuccess, resp.TotalFailed)
		return false
	}

	return true
}

// Проверяем защиту от Path Traversal
func test4() bool {
	validPNG := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("b"), 100)...)
	req, _ := createMultipartRequest([]fileDef{
		{name: "../../../hacked.png", content: validPNG},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	if w.Code != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Тест 4: ожидался статус 201, получен %d\n", w.Code)
		return false
	}

	var resp UploadResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.URLs) != 1 {
		fmt.Fprintf(os.Stderr, "Тест 4: ожидался 1 URL, получено %d\n", len(resp.URLs))
		return false
	}

	fileName := strings.TrimPrefix(resp.URLs[0], "/uploads/")
	savedPath := filepath.Join("uploads", fileName)
	savedContent, err := os.ReadFile(savedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Тест 4: файл не найден по пути %s - проверьте Path Traversal защиту\n", savedPath)
		return false
	}

	if !bytes.Equal(savedContent, validPNG) {
		fmt.Fprintf(os.Stderr, "Тест 4: содержимое файла не совпадает\n")
		return false
	}

	// Проверяем, что файл не сохранился под оригинальным именем
	if fileName == "hacked.png" || strings.Contains(fileName, "..") {
		fmt.Fprintf(os.Stderr, "Тест 4: имя файла %s небезопасно\n", fileName)
		return false
	}

	return true
}

// Проверяем MaxBytesReader, тело запроса более 10MB должно обрываться
func test5() bool {
	bigContent := make([]byte, 11*1024*1024)

	req, _ := createMultipartRequest([]fileDef{
		{name: "huge_payload.png", content: bigContent},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	if w.Code < 400 {
		fmt.Fprintf(os.Stderr, "Тест 5: ожидалась ошибка (статус >= 400) при превышении общего размера запроса в 10MB (MaxBytesReader), получен статус %d\n", w.Code)
		return false
	}

	return true
}

// Проверяем, что хэндлер строго проверяет MIME-тип и отклоняет GIF
func test6() bool {
	validGIF := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x01D\x00;")
	req, _ := createMultipartRequest([]fileDef{
		{name: "cat.gif", content: validGIF},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		fmt.Fprintf(os.Stderr, "Тест 6: ожидался статус 422 при попытке загрузить GIF (допустимы только jpeg и png), получен %d\n", w.Code)
		return false
	}

	var resp UploadResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Failed) != 1 || !strings.Contains(resp.Failed[0], "invalid file type") {
		fmt.Fprintf(os.Stderr, "Тест 6: ожидалась ошибка 'invalid file type' для GIF, получено: %v\n", resp.Failed)
		return false
	}
	return true
}

// Проверяем, что пустые слайсы в JSON сериализуются как [] а не null
func test7() bool {
	fakeJPG := []byte("MZ\x90\x00\x00")
	req, _ := createMultipartRequest([]fileDef{
		{name: "virus.jpg", content: fakeJPG},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		fmt.Fprintf(os.Stderr, "Тест 7: ошибка парсинга JSON: %v\n", err)
		return false
	}

	uploaded, ok := raw["uploaded"].([]any)
	if !ok || uploaded == nil {
		fmt.Fprintf(os.Stderr, "Тест 7: поле 'uploaded' в JSON равно null или отсутствует, ожидался пустой массив []. Инициализируйте слайсы!\n")
		return false
	}

	urls, ok := raw["urls"].([]any)
	if !ok || urls == nil {
		fmt.Fprintf(os.Stderr, "Тест 7: поле 'urls' в JSON равно null или отсутствует, ожидался пустой массив []. Инициализируйте слайсы!\n")
		return false
	}

	return true
}

// Проверяем, что файл размером ровно 1 MB проходит проверку
func test8() bool {
	exactlyOneMB := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("c"), 1024*1024-8)...)
	req, _ := createMultipartRequest([]fileDef{
		{name: "exactly_1mb.png", content: exactlyOneMB},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	if w.Code != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Тест 8: ожидался статус 201 для файла ровно 1 MB, получен %d\n", w.Code)
		return false
	}

	var resp UploadResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TotalSuccess != 1 {
		fmt.Fprintf(os.Stderr, "Тест 8: файл размером ровно 1 MB должен проходить. TotalSuccess=%d\n", resp.TotalSuccess)
		return false
	}
	return true
}

// Проверяем, что хэндлер возвращает корректный Content-Type
func test9() bool {
	validPNG := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("d"), 100)...)
	req, _ := createMultipartRequest([]fileDef{
		{name: "check_ct.png", content: validPNG},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		fmt.Fprintf(os.Stderr, "Тест 9: ожидался Content-Type 'application/json', получен '%s'\n", ct)
		return false
	}
	return true
}

// Проверяем, что GET-запрос корректно обрабатывается (возвращает 400, а не панику или 500)
func test10() bool {
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	w := httptest.NewRecorder()

	UploadsHandler(w, req)

	if w.Code < 400 || w.Code >= 500 {
		fmt.Fprintf(os.Stderr, "Тест 10: для GET-запроса ожидался статус 4xx, получен %d\n", w.Code)
		return false
	}
	return true
}

// Проверяем, что невалидные файлы не остаются на диске
func test11() bool {
	fakeJPG := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF")
	req, _ := createMultipartRequest([]fileDef{
		{name: "should_not_exist.jpg", content: fakeJPG},
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	// Проверяем, что в директории uploads нет файлов с именем should_not_exist
	entries, _ := os.ReadDir("uploads")
	for _, e := range entries {
		if strings.Contains(e.Name(), "should_not_exist") {
			fmt.Fprintf(os.Stderr, "Тест 11: невалидный файл %s не должен был сохраниться на диск\n", e.Name())
			return false
		}
	}
	return true
}

// Проверяем, что сохраненный файл имеет правильное расширение
func test12() bool {
	validPNG := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte("e"), 50)...)
	req, _ := createMultipartRequest([]fileDef{
		{name: "photo.PNG", content: validPNG}, // Большое расширение
	})

	w := httptest.NewRecorder()
	UploadsHandler(w, req)

	var resp UploadResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.URLs) != 1 {
		fmt.Fprintf(os.Stderr, "Тест 12: ожидался 1 URL, получено %d\n", len(resp.URLs))
		return false
	}

	fileName := strings.TrimPrefix(resp.URLs[0], "/uploads/")
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".png" {
		fmt.Fprintf(os.Stderr, "Тест 12: ожидалось расширение .png, получено '%s'\n", ext)
		return false
	}
	return true
}
