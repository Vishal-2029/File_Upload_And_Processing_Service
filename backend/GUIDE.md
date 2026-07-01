# Developer Guide — File Upload & Processing Service

A complete practical reference for running, using, and integrating with the service.

---

## Requirements

| Tool | Version |
|------|---------|
| Docker & Docker Compose | 24+ |
| Go | 1.23+ |
| Make | any |
| golangci-lint | 1.57+ (optional, for `make lint`) |

---

## Quick Start

```bash
# 1. Clone the repository
git clone <repo-url> && cd File_Upload_And_Processing_Service

# 2. Copy environment config
cp .env.example .env

# 3. Start all services (Postgres, MinIO, MailHog, API)
make docker-up

# 4. Seed test data
make seed
```

The API is now available at `http://localhost:3000`.

---

## Services & Ports

| Service | URL | Purpose |
|---------|-----|---------|
| API | `http://localhost:3000` | REST API + WebSocket |
| MinIO S3 API | `http://localhost:9000` | Object storage |
| MinIO Console | `http://localhost:9001` | Storage web UI |
| MailHog | `http://localhost:8025` | Email catch-all (dev) |
| PostgreSQL | `localhost:5432` | Database |
| Redis | `localhost:6379` | Reserved for future use |

---

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `:3000` | Server listen address |
| `JWT_SECRET` | `change-me-in-production` | Secret key for signing JWTs — **change this** |
| `JWT_EXPIRY_HOURS` | `72` | Token lifetime in hours |
| `POSTGRES_DSN` | `postgres://postgres:postgres@localhost:5432/fileservice?sslmode=disable` | PostgreSQL connection string |
| `MINIO_ENDPOINT` | `localhost:9000` | MinIO S3 API endpoint |
| `MINIO_ACCESS_KEY` | `minioadmin` | MinIO access key |
| `MINIO_SECRET_KEY` | `minioadmin` | MinIO secret key |
| `MINIO_BUCKET` | `uploads` | Bucket name (auto-created on startup) |
| `MINIO_USE_SSL` | `false` | Use HTTPS for MinIO |
| `SMTP_HOST` | `localhost` | SMTP server hostname |
| `SMTP_PORT` | `1025` | SMTP server port |
| `SMTP_FROM` | `noreply@fileservice.dev` | Sender address for notifications |
| `TMP_DIR` | `/tmp/fileservice` | Temporary upload directory |
| `PROCESSED_DIR` | `/tmp/fileservice/processed` | Processed file staging directory |
| `MAX_FILE_SIZE_MB` | `50` | Maximum upload size in MB |
| `WORKER_COUNT` | `10` | Number of concurrent processing workers |
| `JOB_QUEUE_SIZE` | `1000` | Buffered job queue capacity |
| `RATE_LIMIT_RPS` | `1.67` | Upload rate limit per user (requests/second) |
| `RATE_LIMIT_BURST` | `10` | Burst allowance for rate limiting |

---

## API Reference

All protected endpoints require a Bearer token in the `Authorization` header. Get a token from `/auth/register` or `/auth/login`.

```bash
# Save your token after login/register
export TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

---

### POST /auth/register

Register a new user account and receive a JWT.

**Auth:** None

**Request body:**
```json
{
  "email": "user@example.com",
  "password": "securepass123"
}
```

**Validation:**
- `email` and `password` are required
- `password` must be at least 8 characters
- `email` must be unique

**curl example:**
```bash
curl -s -X POST http://localhost:3000/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"securepass123"}'
```

**Success — 201 Created:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Error responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid request body"}` |
| 400 | `{"error": "email and password are required"}` |
| 400 | `{"error": "password must be at least 8 characters"}` |
| 409 | `{"error": "email already registered"}` |
| 500 | `{"error": "registration failed"}` |

---

### POST /auth/login

Authenticate and receive a JWT.

**Auth:** None

**Request body:**
```json
{
  "email": "user@example.com",
  "password": "securepass123"
}
```

**curl example:**
```bash
curl -s -X POST http://localhost:3000/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"securepass123"}'
```

**Success — 200 OK:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Error responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid request body"}` |
| 401 | `{"error": "invalid email or password"}` |
| 500 | `{"error": "login failed"}` |

---

### POST /upload

Upload a file for async processing. Returns immediately with a file ID — use the ID to poll status or watch WebSocket events.

**Auth:** Required
**Rate limit:** 1.67 req/s per user, burst of 10

**Request:** `multipart/form-data` with a `file` field.

**Validation:**
- `file` field is required
- File size must not exceed `MAX_FILE_SIZE_MB` (default 50 MB)
- MIME type must be one of the [accepted types](#accepted-file-types)

**curl example:**
```bash
curl -s -X POST http://localhost:3000/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@/path/to/document.pdf"
```

**Success — 202 Accepted:**
```json
{
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "pending"
}
```

**Error responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "file field is required"}` |
| 401 | `{"error": "missing authorization header"}` |
| 401 | `{"error": "invalid or expired token"}` |
| 413 | `{"error": "file exceeds maximum size of 50 MB"}` |
| 415 | `{"error": "unsupported file type", "detected": "text/plain"}` |
| 429 | `{"error": "rate limit exceeded", "retry_after": 60}` + `Retry-After: 60` header |
| 500 | `{"error": "failed to read upload"}` |
| 500 | `{"error": "failed to save upload"}` |
| 500 | `{"error": "failed to create file record"}` |
| 503 | `{"error": "server busy, retry shortly"}` + `Retry-After: 30` header |

---

### GET /files

List all files belonging to the authenticated user, with pagination.

**Auth:** Required

**Query parameters:**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `page` | `1` | Page number |
| `limit` | `20` | Results per page (max 100) |

**curl example:**
```bash
curl -s "http://localhost:3000/files?page=1&limit=20" \
  -H "Authorization: Bearer $TOKEN"
```

**Success — 200 OK:**
```json
{
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "660e8400-e29b-41d4-a716-446655440001",
      "status": "done",
      "file_type": "image",
      "original_name": "photo.jpg",
      "storage_path": "660e8400.../550e8400....jpg",
      "meta": {
        "width": 1920,
        "height": 1080,
        "format": "jpeg"
      },
      "retry_count": 0,
      "created_at": "2026-03-19T10:30:00Z",
      "updated_at": "2026-03-19T10:32:15Z"
    }
  ],
  "total": 42,
  "page": 1,
  "limit": 20
}
```

**Error responses:**

| Status | Body |
|--------|------|
| 401 | `{"error": "missing authorization header"}` |
| 401 | `{"error": "invalid or expired token"}` |
| 500 | `{"error": "failed to list files"}` |

---

### GET /files/:id

Get details for a single file by ID.

**Auth:** Required

**curl example:**
```bash
curl -s http://localhost:3000/files/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer $TOKEN"
```

**Success — 200 OK:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "660e8400-e29b-41d4-a716-446655440001",
  "status": "done",
  "file_type": "pdf",
  "original_name": "document.pdf",
  "storage_path": "660e8400.../550e8400....pdf",
  "meta": {
    "page_count": 12,
    "word_count": 3456
  },
  "retry_count": 0,
  "created_at": "2026-03-19T10:25:00Z",
  "updated_at": "2026-03-19T10:28:45Z"
}
```

**Error responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid file id"}` |
| 401 | `{"error": "invalid or expired token"}` |
| 403 | `{"error": "access denied"}` |
| 404 | `{"error": "file not found"}` |
| 500 | `{"error": "internal server error"}` |

---

### GET /files/:id/download

Get a time-limited presigned URL to download the processed file directly from MinIO. The URL is valid for **60 minutes**.

**Auth:** Required
**Note:** Returns 500 if the file has not finished processing yet.

**curl example:**
```bash
curl -s http://localhost:3000/files/550e8400-e29b-41d4-a716-446655440000/download \
  -H "Authorization: Bearer $TOKEN"
```

**Success — 200 OK:**
```json
{
  "url": "http://localhost:9000/uploads/660e8400.../550e8400....jpg?X-Amz-Signature=...",
  "expires_in": "3600s"
}
```

**Error responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid file id"}` |
| 401 | `{"error": "invalid or expired token"}` |
| 403 | `{"error": "access denied"}` |
| 404 | `{"error": "file not found"}` |
| 500 | `{"error": "file not yet processed"}` |
| 500 | `{"error": "internal server error"}` |

---

### DELETE /files/:id

Permanently delete a file record and its object from MinIO.

**Auth:** Required

**curl example:**
```bash
curl -s -X DELETE http://localhost:3000/files/550e8400-e29b-41d4-a716-446655440000 \
  -H "Authorization: Bearer $TOKEN"
```

**Success — 204 No Content** *(no response body)*

**Error responses:**

| Status | Body |
|--------|------|
| 400 | `{"error": "invalid file id"}` |
| 401 | `{"error": "invalid or expired token"}` |
| 403 | `{"error": "access denied"}` |
| 404 | `{"error": "file not found"}` |
| 500 | `{"error": "internal server error"}` |

---

### GET /health

Health check endpoint.

**Auth:** None

**curl example:**
```bash
curl -s http://localhost:3000/health
```

**Success — 200 OK:**
```json
{
  "status": "ok"
}
```

---

## WebSocket Guide

### Connect

```
ws://localhost:3000/ws?token=<jwt>
```

Pass your JWT as the `token` query parameter. The server validates the token on upgrade — the connection is closed immediately if the token is missing or expired.

**JavaScript example:**
```javascript
const token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...";
const ws = new WebSocket(`ws://localhost:3000/ws?token=${token}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log(msg.event, msg);
};
```

**wscat example:**
```bash
wscat -c "ws://localhost:3000/ws?token=$TOKEN"
```

### Events

The server pushes JSON events when files finish processing. You only receive events for your own files.

**`file.processed`** — file processed successfully:
```json
{
  "event": "file.processed",
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "done",
  "meta": {
    "page_count": 12,
    "word_count": 3456
  }
}
```

For images, `meta` contains:
```json
{
  "width": 1920,
  "height": 1080,
  "format": "jpeg"
}
```

**`file.error`** — processing failed after all retries:
```json
{
  "event": "file.error",
  "file_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "error",
  "error": "PDF processing: invalid PDF structure"
}
```

### Ping / Pong

The server sends a WebSocket ping every **54 seconds**. Clients must respond with a pong within **60 seconds** or the connection is closed. Standard WebSocket clients handle this automatically.

| Parameter | Value |
|-----------|-------|
| Ping interval | 54 s |
| Pong deadline | 60 s |
| Write deadline | 10 s |
| Max inbound message | 512 bytes |
| Per-client send buffer | 256 messages |

---

## File Processing Flow

```
1. Upload (POST /upload)
   └─ File written atomically to TMP_DIR
   └─ DB record created: status = "pending"
   └─ Job enqueued to buffered channel
   └─ 202 response returned immediately

2. Worker dequeues job
   └─ Worker acquires semaphore slot (bounded concurrency)
   └─ DB row locked with SELECT FOR UPDATE
   └─ Guard: if status is already "done" or "error", skip (idempotent)
   └─ DB record updated: status = "processing"

3. Processing (by file type)
   ├─ PDF: extract page count + word count via pdfcpu
   └─ Image: auto-orient → resize to 800×800px (aspect preserved) → save as JPEG 85%

4. Upload to MinIO
   └─ Storage key: {userID}/{fileID}.{ext}
   └─ Temp file deleted after successful upload

5. DB update
   └─ status = "done"
   └─ storage_path populated
   └─ meta updated with processing results

6. Notifications (non-blocking)
   ├─ WebSocket: "file.processed" event pushed to client
   └─ Email: fire-and-forget goroutine sends HTML email via SMTP

--- On any processing error ---

7. Retry (max 3 attempts)
   └─ retry_count incremented
   └─ Exponential backoff: 2s → 4s → 8s
   └─ Job re-enqueued after delay

8. After max retries exceeded
   └─ Row inserted into dead_letter_jobs
   └─ status = "error", meta.error = error message
   ├─ WebSocket: "file.error" event pushed
   └─ Email notification sent
```

---

## Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Compile binary to `bin/server` |
| `make run` | Run the server locally with `go run` |
| `make tidy` | Clean up `go.mod` and `go.sum` |
| `make test` | Run all tests |
| `make test-race` | Run tests with the Go race detector |
| `make test-cover` | Generate and open HTML coverage report |
| `make lint` | Run `golangci-lint` |
| `make docker-up` | Start the full Docker stack |
| `make docker-down` | Stop the stack and remove volumes |
| `make docker-logs` | Stream API container logs |
| `make seed` | Run `scripts/seed.sh` to insert test data |
| `make load-test` | Fire 100 concurrent uploads at `localhost:3000` |

---

## Database Tables

### `users`

| Column | Type | Notes |
|--------|------|-------|
| `id` | UUID | Primary key |
| `email` | VARCHAR(255) | Unique, not null |
| `password` | VARCHAR(255) | bcrypt hash |
| `created_at` | TIMESTAMPTZ | Auto-set |
| `updated_at` | TIMESTAMPTZ | Auto-updated via trigger |

### `files`

| Column | Type | Notes |
|--------|------|-------|
| `id` | UUID | Primary key |
| `user_id` | UUID | FK → users(id) ON DELETE CASCADE |
| `status` | VARCHAR(20) | `pending` \| `processing` \| `done` \| `error` |
| `file_type` | VARCHAR(10) | `pdf` \| `image` |
| `original_name` | VARCHAR(500) | Original filename from upload |
| `storage_path` | VARCHAR(1000) | MinIO object key (null until done) |
| `meta` | JSONB | Processing results (see below) |
| `retry_count` | INT | Default 0 |
| `created_at` | TIMESTAMPTZ | Auto-set |
| `updated_at` | TIMESTAMPTZ | Auto-updated via trigger |

Indexes: `idx_files_user_id`, `idx_files_status`, `idx_files_user_id_created`

**`meta` JSONB shape by file type:**

```json
// PDF
{ "page_count": 12, "word_count": 3456 }

// Image
{ "width": 1920, "height": 1080, "format": "jpeg" }

// On error
{ "error": "PDF processing: invalid structure" }
```

### `dead_letter_jobs`

| Column | Type | Notes |
|--------|------|-------|
| `id` | UUID | Primary key |
| `file_id` | UUID | FK → files(id) |
| `user_id` | UUID | Denormalized for fast lookup |
| `error_msg` | TEXT | Last error description |
| `retry_count` | INT | Number of attempts made |
| `created_at` | TIMESTAMPTZ | When job was dead-lettered |

---

## Accepted File Types

| MIME Type | Stored As | `file_type` |
|-----------|-----------|-------------|
| `application/pdf` | `.pdf` | `pdf` |
| `image/jpeg` | `.jpg` | `image` |
| `image/png` | `.jpg` | `image` |
| `image/gif` | `.jpg` | `image` |
| `image/webp` | `.jpg` | `image` |

MIME type is detected from the first 512 bytes of the file content — not the file extension. All images are converted to JPEG during processing.

---

## Error Reference

| Code | Meaning | Common causes |
|------|---------|---------------|
| 400 | Bad Request | Invalid JSON, missing required fields, non-UUID `:id` |
| 401 | Unauthorized | Missing `Authorization` header, expired or invalid JWT |
| 403 | Forbidden | Attempting to access another user's file |
| 404 | Not Found | File ID does not exist |
| 409 | Conflict | Email already registered |
| 413 | Payload Too Large | File exceeds `MAX_FILE_SIZE_MB` |
| 415 | Unsupported Media Type | MIME type not in allowed list |
| 429 | Too Many Requests | Upload rate limit exceeded; check `Retry-After` header |
| 500 | Internal Server Error | Database, storage, or file system failure |
| 503 | Service Unavailable | Job queue at capacity (`JOB_QUEUE_SIZE`); check `Retry-After: 30` header |
