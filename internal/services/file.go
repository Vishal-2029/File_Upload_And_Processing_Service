package services

import (
	"context"
	"errors"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	repointerfacer "github.com/Vishal-2029/file-upload-service/internal/repo/interfaces"
	"github.com/Vishal-2029/file-upload-service/internal/storage"
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
