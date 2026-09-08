# Paranormal-HTTP

HTTP API для досье на паранормальные сущности и загрузки фотографий-доказательств.

## Запуск

Требуется Go 1.26.2 или новее.

```sh
go run . -addr :8080 -storage ./storage
```

Каталоги `entities` и `evidence` создаются автоматически внутри хранилища.
При SIGINT или SIGTERM сервер завершает активные запросы в течение 10 секунд.

## Структура

```text
main.go                  — вызов app.Main и установка кода завершения
internal/
  app/cli.go             — флаги, логгер и обработка сигналов
  app/app.go             — сборка зависимостей и жизненный цикл сервера
  config/config.go       — конфигурация командной строки
  model/entity.go        — модели досье
  model/validation.go    — правила проверки досье
  middleware/
    recovery.go          — перехват паник, независимо от обработчиков
    recovery_test.go     — проверка recovery
  service/
    entity.go            — создание досье, частичный успех и откат файлов
    entity_test.go       — проверка отката при ошибке сохранения
  httpapi/
    server.go            — зависимости HTTP-обработчиков
    router.go            — маршруты и подключение middleware
    entity.go            — создание и получение досье
    evidence.go          — выдача доказательств с поддержкой Range
    response.go          — модели ответов и JSON
    server_test.go       — проверки HTTP API
    router_test.go       — проверка запросов через маршрутизатор
  storage/
    storage.go           — создание хранилища и чтение файлов
    entity.go            — сохранение досье через временный файл
    evidence.go          — проверка, сохранение и откат загрузок
    entity_test.go       — проверка сохранения досье
```

`app` запускает HTTP API и хранилище. Обработчики разбирают HTTP-запросы,
вызывают `service` для создания досье и формируют ответы. Сервис управляет
сохранением доказательств и откатом при ошибках; `storage` отвечает за файлы.
Правила проверки досье находятся в `model`. Пакет `middleware` не зависит
от обработчиков и подключается в маршрутизаторе.
Данные хранятся отдельно от исходников,
по умолчанию в `./storage`.

## API

| Метод | Путь | Описание |
| --- | --- | --- |
| POST | `/api/v1/entities` | Создать досье: multipart-поля `dossier` (JSON) и `evidence` (файлы) |
| GET | `/api/v1/entities/{id}` | Получить досье по UUID |
| GET | `/api/v1/evidence/{filename}` | Получить файл доказательства |

Досье содержит `name`, `description`, `threat_level` (1–10) и `vulnerabilities`.
Нужно приложить от 1 до 10 файлов JPEG/PNG, каждый не более 3 МиБ.
Лимит тела запроса — 20 МиБ, JSON досье — 1 МиБ.
Успешное создание возвращает 201, частичное сохранение доказательств — 207.

Пример из PowerShell:

```powershell
curl.exe -F 'dossier=<dossier.json' -F 'evidence=@testdata/cat.jpg' http://localhost:8080/api/v1/entities
```

## Проверки

```sh
go test ./...
go vet ./...
```
