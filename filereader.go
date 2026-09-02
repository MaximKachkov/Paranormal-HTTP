/*package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

// Вспомогательная функция для генерации уникального имени
func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func UploadHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	defer r.Body.Close()

	err := r.ParseMultipartForm(1 << 20)
	if err != nil {
		http.Error(w, "", 413)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if err == http.ErrMissingFile {
			http.Error(w, "", http.StatusBadRequest)
			return
		} else {
			http.Error(w, "", 413)
			return
		}
	}
	defer file.Close()
	allowedExtensions := map[string]struct{}{
		".jpg": {}, ".jpeg": {}, ".png": {},
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if _, ok := allowedExtensions[ext]; !ok {
		http.Error(w, "", 400)
		return
	}

	buffer := make([]byte, 512)

	if _, err := file.Read(buffer); err != nil {
		http.Error(w, "", 400)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "", 500)
		return
	}
	mimeType := http.DetectContentType(buffer)

	allowedMimeTypes := map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
		"image/jpg":  {},
	}
	if _, ok := allowedMimeTypes[mimeType]; !ok {
		http.Error(w, "", 400)
		return
	}

	uniqueID := generateID()
	newFileName := uniqueID + ext
	TempDir := os.TempDir()
	newFilePath := filepath.Join(TempDir, newFileName)
	newfile, err := os.Create(newFilePath)
	if err != nil {
		http.Error(w, "", 500)
		return
	}
	_, err = io.Copy(newfile, file)
	if err != nil {
		http.Error(w, "", 500)
		os.Remove(newFilePath)
		return
	}
	defer newfile.Close()
	w.WriteHeader(http.StatusCreated)
	response := map[string]string{
		"url": newFilePath,
	}
	json.NewEncoder(w).Encode(response)

}

// Функция main и все тесты будут скрыты от вас при проверке на сайте.
func main() {
	// Если вы захотите запустить сервер локально для проверки: RUN=1 go run main.go
	if os.Getenv("RUN") == "1" {
		http.HandleFunc("/upload", UploadHandler)
		fmt.Println("Сервер запущен локально на http://localhost:8080/upload")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Fprintf(os.Stderr, "Ошибка запуска сервера: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if !test1() || !test2() || !test3() || !test4() || !test5() || !test6() || !test7() {
		os.Exit(1)
	}
	fmt.Println("Все тесты успешно пройдены!")
}

// createMultipartRequest генерирует multipart-запросы в тестах
func createMultipartRequest(fieldname, filename string, content []byte) (*http.Request, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	if fieldname != "" {
		part, err := writer.CreateFormFile(fieldname, filename)
		if err != nil {
			return nil, err
		}
		part.Write(content)
	}
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, nil
}

// Проверяем валидную загрузку png и проверяем момент с забытым Seek(0)
func test1() bool {
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	content := append(pngHeader, bytes.Repeat([]byte{0xAA}, 1000)...)

	req, _ := createMultipartRequest("file", "test.png", content)
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Тест 1: Ожидался статус 201 для валидного файла, получен %d\n", rr.Code)
		return false
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Тест 1: Неверный формат JSON ответа: %v\n", err)
		return false
	}

	urlPath, ok := resp["url"]
	if !ok {
		fmt.Fprintf(os.Stderr, "Тест 1: В JSON ответе отсутствует поле 'url'\n")
		return false
	}

	savedFilename := filepath.Base(urlPath)
	savedPath := filepath.Join(os.TempDir(), savedFilename)

	savedContent, err := os.ReadFile(savedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Тест 1: Не удалось найти сохраненный файл на диске: %v\n", err)
		return false
	}

	// Проверяем, что размер не меньше исходного (забытый Seek съест первые 512 байт)
	if len(savedContent) != len(content) {
		fmt.Fprintf(os.Stderr, "Тест 1: Размер сохраненного файла (%d байт) не совпадает с исходным (%d байт). Вы забыли сделать file.Seek(0, io.SeekStart) после проверки MIME?\n", len(savedContent), len(content))
		return false
	}

	// Строгая сверка байтов
	if !bytes.Equal(savedContent, content) {
		fmt.Fprintf(os.Stderr, "Тест 1: Содержимое сохраненного файла повреждено (байты не совпадают).\n")
		return false
	}

	return true
}

// Проверяем валидную загрузку jpeg и регистронезависимость расширения
func test2() bool {
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
	content := append(jpegHeader, bytes.Repeat([]byte{0xBB}, 1000)...)

	req, _ := createMultipartRequest("file", "PICTURE.JpEg", content)
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Тест 2: Ожидался статус 201 для валидного файла .JpEg, получен %d. Проверьте регистронезависимость!\n", rr.Code)
		return false
	}
	return true
}

// Проверяем обработку некорректного имени поля в форме, отсутствует 'file'
func test3() bool {
	req, _ := createMultipartRequest("wrong_field", "test.png", []byte("some data"))
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		fmt.Fprintf(os.Stderr, "Тест 4: Ожидался статус 400 при отсутствии поля 'file', получен %d\n", rr.Code)
		return false
	}
	return true
}

// Файл с корректной сигнатурой, но плохим расширением
func test4() bool {
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	content := append(pngHeader, bytes.Repeat([]byte{0xAA}, 1000)...)

	req, _ := createMultipartRequest("file", "image.gif", content)
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		fmt.Fprintf(os.Stderr, "Тест 5: Ожидался статус 400 для файла с неразрешенным расширением .gif (в white-list только jpg, jpeg, png), получен %d\n", rr.Code)
		return false
	}
	return true
}

// Подделка расширения, плохой контент, но хорошее расширение
func test5() bool {
	content := []byte("<html><body>You are hacked!</body></html>")
	content = append(content, bytes.Repeat([]byte{0x20}, 1000)...) // добиваем размер > 512 байт

	req, _ := createMultipartRequest("file", "evil.png", content)
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		fmt.Fprintf(os.Stderr, "Тест 6: Ожидался статус 400. Злоумышленник отправил html-файл с расширением .png. DetectContentType должен был это заблокировать!\n")
		return false
	}
	return true
}

// Проверяем защиту от Directory Traversal атак в имени файла
func test6() bool {
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	content := append(pngHeader, bytes.Repeat([]byte{0xAA}, 1000)...)

	// Злоумышленник пытается выйти из директории загрузок
	req, _ := createMultipartRequest("file", "../../../etc/shadow.png", content)
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "Тест 7: Ожидался статус 201 для файла с вредоносным именем. Вы должны были очистить имя и сохранить его.\n")
		return false
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	urlPath := resp["url"]

	if strings.Contains(urlPath, "../") || strings.Contains(urlPath, "etc") || strings.Contains(urlPath, "shadow") {
		fmt.Fprintf(os.Stderr, "Тест 7: В итоговом URL остался мусор от попытки Directory Traversal. Используйте генерацию уникального ID и извлекайте только расширение.\n")
		return false
	}

	return true
}

// Проверяем работу MaxBytesReader на больших файлах (> 5 МБ)
func test7() bool {
	content := make([]byte, 5*1024*1024+1024) // 5 МБ + 1 КБ
	req, _ := createMultipartRequest("file", "big.png", content)
	rr := httptest.NewRecorder()
	UploadHandler(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		fmt.Fprintf(os.Stderr, "Тест 8: Ожидался статус 413 для файла размером более 5МБ, получен %d\n", rr.Code)
		return false
	}
	return true
}
