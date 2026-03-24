package handlers

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/Vishal-2029/file-upload-service/internal/services"
	svcinterfaces "github.com/Vishal-2029/file-upload-service/internal/services/interfaces"
)

type FileHandler struct {
	fileSvc svcinterfaces.FileService
}

func NewFileHandler(router fiber.Router, fileSvc svcinterfaces.FileService) {
	h := &FileHandler{fileSvc: fileSvc}
	router.Get("/files", h.List)
	router.Get("/files/:id", h.Get)
	router.Get("/files/:id/download", h.Download)
	router.Delete("/files/:id", h.Delete)
}

// List godoc
// GET /files?page=1&limit=20
func (h *FileHandler) List(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))

	files, total, err := h.fileSvc.List(c.Context(), userID, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list files"})
	}

	return c.JSON(fiber.Map{
		"data":  files,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Get godoc
// GET /files/:id
func (h *FileHandler) Get(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	file, err := h.fileSvc.GetByID(c.Context(), userID, fileID)
	if err != nil {
		return fileError(c, err)
	}

	return c.JSON(file)
}

// Download godoc
// GET /files/:id/download — returns a 60-minute presigned MinIO URL
func (h *FileHandler) Download(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	presignedURL, err := h.fileSvc.PresignedURL(c.Context(), userID, fileID, 60*time.Minute)
	if err != nil {
		return fileError(c, err)
	}

	return c.JSON(fiber.Map{"url": presignedURL, "expires_in": "3600s"})
}

// Delete godoc
// DELETE /files/:id
func (h *FileHandler) Delete(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	if err := h.fileSvc.Delete(c.Context(), userID, fileID); err != nil {
		return fileError(c, err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func mustParseUserID(c *fiber.Ctx) uuid.UUID {
	id, _ := c.Locals("userID").(string)
	uid, _ := uuid.Parse(id)
	return uid
}

func fileError(c *fiber.Ctx, err error) error {
	if errors.Is(err, services.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "file not found"})
	}
	if errors.Is(err, services.ErrForbidden) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
}
