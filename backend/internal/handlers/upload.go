package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
	"github.com/Vishal-2029/file-upload-service/internal/queue"
	svcinterfaces "github.com/Vishal-2029/file-upload-service/internal/services/interfaces"
)

type UploadHandler struct {
	fileSvc      svcinterfaces.FileService
	jobQueue     *queue.JobQueue
	tmpDir       string
	maxSizeBytes int64
}

func NewUploadHandler(
	router fiber.Router,
	fileSvc svcinterfaces.FileService,
	jobQueue *queue.JobQueue,
	tmpDir string,
	maxFileSizeMB int64,
) {
	h := &UploadHandler{
		fileSvc:      fileSvc,
		jobQueue:     jobQueue,
		tmpDir:       tmpDir,
		maxSizeBytes: maxFileSizeMB * 1024 * 1024,
	}
	router.Post("/upload", h.Upload)
}

func (h *UploadHandler) Upload(c *fiber.Ctx) error {
	userID, _ := c.Locals("userID").(string)
	userEmail, _ := c.Locals("userEmail").(string)

	uid, err := uuid.Parse(userID)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid user"})
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file field is required"})
	}

	if fileHeader.Size > h.maxSizeBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{
			"error": fmt.Sprintf("file exceeds maximum size of %d MB", h.maxSizeBytes/1024/1024),
		})
	}

	// Open file to read MIME type from first 512 bytes.
	src, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read upload"})
	}
	defer src.Close()

	buf := make([]byte, 512)
	n, err := src.Read(buf)
	if err != nil && err != io.EOF {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read upload"})
	}
	mimeType := http.DetectContentType(buf[:n])

	fileType, ok := domain.AllowedMIMETypes[mimeType]
	if !ok {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"error":    "unsupported file type",
			"detected": mimeType,
		})
	}

	// Write to tmp file atomically: write to .tmp then rename.
	tmpID := uuid.New().String()
	ext := ".pdf"
	if fileType == domain.FileTypeImage {
		ext = ".jpg"
	}
	tmpPath := filepath.Join(h.tmpDir, tmpID+ext)
	tmpPathPartial := tmpPath + ".tmp"

	if err := os.MkdirAll(h.tmpDir, 0o755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to prepare tmp dir"})
	}

	out, err := os.OpenFile(tmpPathPartial, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save upload"})
	}

	// Reset reader to beginning and copy all content (first 512 bytes already read).
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		out.Close()
		os.Remove(tmpPathPartial)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save upload"})
	}

	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		os.Remove(tmpPathPartial)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save upload"})
	}
	out.Close()

	// Atomic rename (same filesystem: /tmp → /tmp).
	if err := os.Rename(tmpPathPartial, tmpPath); err != nil {
		os.Remove(tmpPathPartial)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save upload"})
	}

	// Create DB record (status: pending).
	file, err := h.fileSvc.Create(c.Context(), uid, fileHeader, tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create file record"})
	}

	// Enqueue for async processing — never blocks.
	job := domain.Job{
		FileID:       file.ID,
		UserID:       uid,
		UserEmail:    userEmail,
		TmpPath:      tmpPath,
		FileType:     fileType,
		OriginalName: fileHeader.Filename,
	}

	if err := h.jobQueue.Enqueue(job); err != nil {
		log.Warn().Str("file_id", file.ID.String()).Msg("job queue full, returning 503")
		c.Set("Retry-After", "30")
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"error": "server busy, retry shortly"})
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"file_id": file.ID,
		"status":  domain.StatusPending,
	})
}
