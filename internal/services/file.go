package services

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	repointerfacer "github.com/Vishal-2029/file-upload-service/internal/repo/interfaces"
	"github.com/Vishal-2029/file-upload-service/internal/storage"
	"github.com/Vishal-2029/file-upload-service/internal/worker"
)

var ErrNotFound = errors.New("file not found")
var ErrForbidden = errors.New("access denied")

type FileService struct {
	fileRepo repointerfacer.FileRepo
	storage  *storage.MinioStorage
}

func NewFileService(fileRepo repointerfacer.FileRepo, storage *storage.MinioStorage) *FileService {
	return &FileService{fileRepo: fileRepo, storage: storage}
}

func (s *FileService) Create(ctx context.Context, userID uuid.UUID, header *multipart.FileHeader, tmpPath string) (*models.File, error) {
	ext := strings.ToLower(filepath.Ext(header.Filename))
	fileType := domain.FileTypePDF
	if ext != ".pdf" {
		fileType = domain.FileTypeImage
	}

	file := &models.File{
		ID:           uuid.New(),
		UserID:       userID,
		Status:       domain.StatusPending,
		FileType:     fileType,
		OriginalName: header.Filename,
		Meta:         models.FileMeta{},
	}

	if err := s.fileRepo.Create(ctx, file); err != nil {
		return nil, err
	}
	return file, nil
}

func (s *FileService) List(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.File, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.fileRepo.ListByUser(ctx, userID, page, limit)
}

func (s *FileService) GetByID(ctx context.Context, userID, fileID uuid.UUID) (*models.File, error) {
	file, err := s.fileRepo.FindByID(ctx, fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if file.UserID != userID {
		return nil, ErrForbidden
	}
	return file, nil
}

func (s *FileService) PresignedURL(ctx context.Context, userID, fileID uuid.UUID, expiry time.Duration) (string, error) {
	file, err := s.GetByID(ctx, userID, fileID)
	if err != nil {
		return "", err
	}
	if file.StoragePath == "" {
		return "", errors.New("file not yet processed")
	}

	u, err := s.storage.PresignedGetURL(ctx, file.StoragePath, expiry)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (s *FileService) Delete(ctx context.Context, userID, fileID uuid.UUID) error {
	file, err := s.GetByID(ctx, userID, fileID)
	if err != nil {
		return err
	}

	if file.StoragePath != "" {
		_ = s.storage.RemoveObject(ctx, file.StoragePath)
	}

	return s.fileRepo.Delete(ctx, fileID)
}

// GetText returns the full extracted text of a processed PDF.
func (s *FileService) GetText(ctx context.Context, userID, fileID uuid.UUID) (string, error) {
	file, err := s.GetByID(ctx, userID, fileID)
	if err != nil {
		return "", err
	}
	if file.FileType != domain.FileTypePDF {
		return "", errors.New("text extraction is only available for PDF files")
	}
	if file.Status != domain.StatusDone {
		return "", errors.New("file is not yet processed")
	}
	return file.ExtractedText, nil
}

// GetPages returns the extracted text split into individual pages.
func (s *FileService) GetPages(ctx context.Context, userID, fileID uuid.UUID) ([]worker.PageText, error) {
	text, err := s.GetText(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	return worker.ParsePages(text), nil
}

// UpdatePage replaces the text for a single page, leaving other pages untouched.
func (s *FileService) UpdatePage(ctx context.Context, userID, fileID uuid.UUID, pageNum int, newText string) error {
	file, err := s.GetByID(ctx, userID, fileID)
	if err != nil {
		return err
	}
	if file.FileType != domain.FileTypePDF {
		return errors.New("page editing is only available for PDF files")
	}
	if file.Status != domain.StatusDone {
		return errors.New("file is not yet processed")
	}

	pages := worker.ParsePages(file.ExtractedText)
	updated := false
	for i, p := range pages {
		if p.Page == pageNum {
			pages[i].Text = newText
			updated = true
			break
		}
	}
	if !updated {
		return fmt.Errorf("page %d not found", pageNum)
	}

	// Rebuild the full text blob from updated pages.
	var buf strings.Builder
	for i, p := range pages {
		if i > 0 {
			buf.WriteString(fmt.Sprintf("\n\n[PAGE %d]\n\n", p.Page))
		}
		buf.WriteString(p.Text)
	}
	return s.fileRepo.UpdateExtractedText(ctx, fileID, buf.String())
}

// UpdateText saves the user-edited text for a PDF file.
func (s *FileService) UpdateText(ctx context.Context, userID, fileID uuid.UUID, text string) error {
	file, err := s.GetByID(ctx, userID, fileID)
	if err != nil {
		return err
	}
	if file.FileType != domain.FileTypePDF {
		return errors.New("text editing is only available for PDF files")
	}
	if file.Status != domain.StatusDone {
		return errors.New("file is not yet processed")
	}
	return s.fileRepo.UpdateExtractedText(ctx, fileID, text)
}

// ExportPDF generates a new PDF from the current (possibly edited) extracted text,
// uploads it to MinIO, and returns a presigned download URL.
func (s *FileService) ExportPDF(ctx context.Context, userID, fileID uuid.UUID) (string, error) {
	file, err := s.GetByID(ctx, userID, fileID)
	if err != nil {
		return "", err
	}
	if file.FileType != domain.FileTypePDF {
		return "", errors.New("export is only available for PDF files")
	}
	if file.Status != domain.StatusDone {
		return "", errors.New("file is not yet processed")
	}
	if strings.TrimSpace(file.ExtractedText) == "" {
		return "", errors.New("no text content to export")
	}

	// Write generated PDF to a temp file.
	tmpFile, err := os.CreateTemp("", "export-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := worker.GeneratePDF(file.ExtractedText, tmpPath); err != nil {
		return "", fmt.Errorf("generate pdf: %w", err)
	}

	// Store the exported PDF under a separate key so the original is preserved.
	exportKey := fmt.Sprintf("%s/%s_edited.pdf", userID.String(), fileID.String())
	if err := s.storage.PutFile(ctx, exportKey, tmpPath, "application/pdf"); err != nil {
		return "", fmt.Errorf("upload exported pdf: %w", err)
	}

	u, err := s.storage.PresignedGetURL(ctx, exportKey, 60*time.Minute)
	if err != nil {
		return "", fmt.Errorf("presign url: %w", err)
	}
	return u.String(), nil
}
