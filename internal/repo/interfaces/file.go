package interfaces

import (
	"context"

	"github.com/google/uuid"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	"gorm.io/gorm"
)

type FileRepo interface {
	Create(ctx context.Context, file *models.File) error
	FindByID(ctx context.Context, id uuid.UUID) (*models.File, error)
	FindByIDForUpdate(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*models.File, error)
	ListByUser(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.File, int64, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdateDone(ctx context.Context, id uuid.UUID, storagePath string, meta models.FileMeta) error
	UpdateError(ctx context.Context, id uuid.UUID, errMsg string) error
	IncrementRetry(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
}
