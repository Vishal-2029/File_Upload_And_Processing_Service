package repo

import (
	"context"

	"github.com/google/uuid"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FileRepo struct {
	db *gorm.DB
}

func NewFileRepo(db *gorm.DB) *FileRepo {
	return &FileRepo{db: db}
}

func (r *FileRepo) Create(ctx context.Context, file *models.File) error {
	return r.db.WithContext(ctx).Create(file).Error
}

func (r *FileRepo) FindByID(ctx context.Context, id uuid.UUID) (*models.File, error) {
	var f models.File
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&f).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

// FindByIDForUpdate locks the row for the duration of the calling transaction.
func (r *FileRepo) FindByIDForUpdate(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*models.File, error) {
	var f models.File
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&f).Error
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *FileRepo) ListByUser(ctx context.Context, userID uuid.UUID, page, limit int) ([]models.File, int64, error) {
	var files []models.File
	var total int64

	offset := (page - 1) * limit

	if err := r.db.WithContext(ctx).Model(&models.File{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&files).Error; err != nil {
		return nil, 0, err
	}

	return files, total, nil
}

func (r *FileRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return r.db.WithContext(ctx).
		Model(&models.File{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (r *FileRepo) UpdateDone(ctx context.Context, id uuid.UUID, storagePath string, meta models.FileMeta) error {
	return r.db.WithContext(ctx).
		Model(&models.File{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":       "done",
			"storage_path": storagePath,
			"meta":         meta,
		}).Error
}

func (r *FileRepo) UpdateError(ctx context.Context, id uuid.UUID, errMsg string) error {
	return r.db.WithContext(ctx).
		Model(&models.File{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status": "error",
			"meta":   models.FileMeta{Error: errMsg},
		}).Error
}

func (r *FileRepo) IncrementRetry(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&models.File{}).
		Where("id = ?", id).
		UpdateColumn("retry_count", gorm.Expr("retry_count + 1")).Error
}

func (r *FileRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&models.File{}).Error
}

// DB exposes the underlying gorm.DB for transaction use in the processor.
func (r *FileRepo) DB() *gorm.DB {
	return r.db
}
