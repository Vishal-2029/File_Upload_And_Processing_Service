package handlers

import (
	"errors"
	"fmt"
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
	router.Get("/files/:id/raw", h.Raw)
	router.Delete("/files/:id", h.Delete)
	router.Get("/files/:id/text", h.GetText)
	router.Put("/files/:id/text", h.UpdateText)
	router.Post("/files/:id/export-pdf", h.ExportPDF)
	router.Get("/files/:id/pages", h.GetPages)
	router.Put("/files/:id/pages/:page", h.UpdatePage)
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

// Raw godoc
// GET /files/:id/raw — streams the original PDF bytes same-origin (for the
// browser-side overlay editor). Avoids CORS issues with presigned MinIO URLs.
func (h *FileHandler) Raw(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	reader, name, err := h.fileSvc.GetRawObject(c.Context(), userID, fileID)
	if err != nil {
		return fileError(c, err)
	}

	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q`, name))
	c.Set("Cache-Control", "private, max-age=0, no-store")
	return c.SendStream(reader)
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

// GetText godoc
// GET /files/:id/text — returns the extracted text of a processed PDF
func (h *FileHandler) GetText(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	text, err := h.fileSvc.GetText(c.Context(), userID, fileID)
	if err != nil {
		return fileError(c, err)
	}

	return c.JSON(fiber.Map{"file_id": fileID, "text": text})
}

// UpdateText godoc
// PUT /files/:id/text — saves user-edited text for a PDF
func (h *FileHandler) UpdateText(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	var body struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Text == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "text cannot be empty"})
	}

	if err := h.fileSvc.UpdateText(c.Context(), userID, fileID, body.Text); err != nil {
		return fileError(c, err)
	}

	return c.JSON(fiber.Map{"message": "text updated successfully"})
}

// ExportPDF godoc
// POST /files/:id/export-pdf — generates a new PDF from the (edited) text and returns a presigned URL
func (h *FileHandler) ExportPDF(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	url, err := h.fileSvc.ExportPDF(c.Context(), userID, fileID)
	if err != nil {
		return fileError(c, err)
	}

	return c.JSON(fiber.Map{"url": url, "expires_in": "3600s"})
}

// GetPages godoc
// GET /files/:id/pages — returns extracted text split into individual pages
func (h *FileHandler) GetPages(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	pages, err := h.fileSvc.GetPages(c.Context(), userID, fileID)
	if err != nil {
		return fileError(c, err)
	}

	return c.JSON(fiber.Map{"file_id": fileID, "total_pages": len(pages), "pages": pages})
}

// UpdatePage godoc
// PUT /files/:id/pages/:page — edit the text of one specific page
func (h *FileHandler) UpdatePage(c *fiber.Ctx) error {
	userID := mustParseUserID(c)

	fileID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid file id"})
	}

	pageNum, err := strconv.Atoi(c.Params("page"))
	if err != nil || pageNum < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid page number"})
	}

	var body struct {
		Text string `json:"text"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.fileSvc.UpdatePage(c.Context(), userID, fileID, pageNum, body.Text); err != nil {
		return fileError(c, err)
	}

	return c.JSON(fiber.Map{"message": "page updated successfully", "page": pageNum})
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
