package worker

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// DeadLetterJob is persisted when a file job exhausts all retries.
type DeadLetterJob struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	FileID     uuid.UUID `gorm:"type:uuid;not null"`
	UserID     uuid.UUID `gorm:"type:uuid;not null"`
	ErrorMsg   string
	RetryCount int
	CreatedAt  time.Time
}

// DeadLetter persists failed jobs to the dead_letter_jobs table.
type DeadLetter struct {
	db *gorm.DB
}

func NewDeadLetter(db *gorm.DB) *DeadLetter {
	return &DeadLetter{db: db}
}

func (dl *DeadLetter) Save(ctx context.Context, fileID, userID uuid.UUID, errMsg string, retryCount int) {
	entry := &DeadLetterJob{
		ID:         uuid.New(),
		FileID:     fileID,
		UserID:     userID,
		ErrorMsg:   errMsg,
		RetryCount: retryCount,
	}
	if err := dl.db.WithContext(ctx).Table("dead_letter_jobs").Create(entry).Error; err != nil {
		log.Error().Err(err).Str("file_id", fileID.String()).Msg("failed to persist dead letter job")
	}
}
