package interfaces

import (
	"context"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	"github.com/Vishal-2029/file-upload-service/internal/worker"
)

type FileService interface {
	Create(ctx context.Context, userID uuid.UUID, header *multipart.FileHeader, tmpPath string) (*models.File, error)
	List(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.File, int64, error)
	GetByID(ctx context.Context, userID, fileID uuid.UUID) (*models.File, error)
	PresignedURL(ctx context.Context, userID, fileID uuid.UUID, expiry time.Duration) (string, error)
	Delete(ctx context.Context, userID, fileID uuid.UUID) error
	GetText(ctx context.Context, userID, fileID uuid.UUID) (string, error)
	UpdateText(ctx context.Context, userID, fileID uuid.UUID, text string) error
	ExportPDF(ctx context.Context, userID, fileID uuid.UUID) (string, error)
	GetPages(ctx context.Context, userID, fileID uuid.UUID) ([]worker.PageText, error)
	UpdatePage(ctx context.Context, userID, fileID uuid.UUID, pageNum int, newText string) error
}
