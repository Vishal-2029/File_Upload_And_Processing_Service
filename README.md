# File Upload & Processing Service

A production-grade Go microservice demonstrating:
- **Bounded worker pool** with semaphore-based concurrency control
- **JWT authentication** with per-user token-bucket rate limiting
- **Async file processing** (PDF text extraction, image resizing) via MinIO object storage
- **Real-time WebSocket notifications** pushed to connected clients
- **Email notifications** on job completion via SMTP
- **Dead-letter queue** for persistently failed jobs


## Architecture

```
HTTP Request
     │
     ▼
[Fiber Router]
     │
     ├── POST /upload ──► [Rate Limiter] ──► [MIME Validate] ──► [Save tmp] ──► [DB: pending] ──► [Enqueue] ──► 202
     │
     └── GET /ws ──────► [JWT query param] ──► [WS Hub] ──► real-time events
                                                    ▲
[Job Queue] ◄──────────────────────────────────────┘
     │
     ▼
[Worker Pool: 10 goroutines + semaphore(10)]
     │
     ├── SELECT FOR UPDATE (DB lock)
     ├── status → processing
     ├── process (PDF or Image)
     ├── upload to MinIO
     ├── status → done + meta
     ├── WS Hub.Send()
     └── email fire-and-forget
           │
           └── (on failure × 3) → dead_letter_jobs
```

---

## Quick Start

### Prerequisites
- Docker & Docker Compose
- Go 1.23+
- Make

### 1. Clone and configure

```bash
cp .env.example .env
# Edit .env if needed (defaults work with docker-compose)
```

### 2. Start infrastructure

```bash
make docker-up
```

This starts: PostgreSQL, MinIO, MailHog, and the API service.

### 3. Verify services

| Service | URL |
|---|---|
| API | http://localhost:3000 |
| MinIO Console | http://localhost:9001 |
| MailHog UI | http://localhost:8025 |

### 4. Run seed script

```bash
make seed
```

---

## API Reference

### Authentication

#### `POST /auth/register`
```json
{ "email": "user@example.com", "password": "secret123" }
```
Response: `{ "token": "eyJ..." }`

#### `POST /auth/login`
```json
{ "email": "user@example.com", "password": "secret123" }
```
Response: `{ "token": "eyJ..." }`

---

### File Operations

All file endpoints require `Authorization: Bearer <token>`.

#### `POST /upload`
Upload a PDF or image (JPEG/PNG/GIF/WebP). Max size controlled by `MAX_FILE_SIZE_MB`.

```bash
curl -X POST http://localhost:3000/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@sample.pdf"
```
Response: `202 Accepted`
```json
{ "file_id": "uuid", "status": "pending" }
```

#### `GET /files`
List files for the authenticated user (paginated).

Query params: `page` (default: 1), `limit` (default: 20, max: 100)

#### `GET /files/:id`
Get file status and metadata.

```json
{
  "id": "uuid",
  "status": "done",
  "file_type": "pdf",
  "original_name": "report.pdf",
  "meta": { "page_count": 12, "word_count": 3847 }
}
```

#### `GET /files/:id/download`
Returns a presigned MinIO URL valid for 60 minutes.

#### `DELETE /files/:id`
Deletes the file from MinIO and the database record.

---

### WebSocket

Connect with JWT token as query param:
```
ws://localhost:3000/ws?token=eyJ...
```

Receive events:
```json
{ "event": "file.processed", "file_id": "uuid", "status": "done", "meta": {...} }
{ "event": "file.error",     "file_id": "uuid", "status": "error", "error": "..." }
```

---

## Development

```bash
make build        # compile binary
make run          # run locally (needs .env)
make test         # run unit tests
make test-race    # run with -race flag
make docker-up    # start all services
make docker-down  # stop all services
make seed         # seed test data
make lint         # golangci-lint
```

---

## Project Structure

```
.
├── cmd/main.go                     # entry point
├── config/config.go                # env config
├── internal/
│   ├── db/postgres.go              # GORM connection
│   ├── di/wire.go                  # dependency injection
│   ├── domain/file.go              # Job struct, status constants
│   ├── handlers/                   # Fiber HTTP handlers
│   ├── middleware/                 # JWT, rate limiting
│   ├── models/                     # GORM models
│   ├── notification/               # WebSocket hub + email
│   ├── queue/job_queue.go          # buffered channel queue
│   ├── repo/                       # GORM repositories
│   ├── services/                   # business logic
│   ├── storage/minio.go            # MinIO wrapper
│   └── worker/                     # pool, processor, pdf, image
├── migrations/001_init.sql         # DB schema
├── deployments/docker-compose.yml  # full stack
└── scripts/seed.sh                 # test data
```

---

## Concurrency Patterns Demonstrated

1. **Bounded worker pool**: 10 dispatcher goroutines + semaphore(10) limits concurrent I/O
2. **Non-blocking enqueue**: `select { case ch <- job: default: return 503 }` — HTTP never blocks
3. **Channel pipeline**: upload handler → queue → pool → processor → hub → client
4. **Atomic tmp write**: `os.OpenFile` → `io.Copy` → `os.Rename` (atomic on same filesystem)
5. **Per-user rate limiting**: `sync.Map` + `x/time/rate` token bucket, auto-evicting
6. **Graceful shutdown**: `os.Signal` → context cancel → `pool.Wait()` → server shutdown
