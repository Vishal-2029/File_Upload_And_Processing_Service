package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	"github.com/Vishal-2029/file-upload-service/internal/notification"
	ws "github.com/Vishal-2029/file-upload-service/internal/notification/ws"
	"github.com/Vishal-2029/file-upload-service/internal/queue"
	"github.com/Vishal-2029/file-upload-service/internal/repo"
	"github.com/Vishal-2029/file-upload-service/internal/storage"
)

const maxRetries = 3

// Processor orchestrates the full processing pipeline for a single job:
//  1. Lock DB row (SELECT FOR UPDATE)
//  2. Set status → processing
//  3. Process file (PDF or image)
//  4. Upload result to MinIO
//  5. Set status → done + store meta
//  6. Send WebSocket notification
//  7. Send email (fire-and-forget)
//
// On failure: exponential backoff re-enqueue up to maxRetries,
// then persist to dead_letter_jobs.
type Processor struct {
	fileRepo     *repo.FileRepo
	storage      *storage.MinioStorage
	hub          *ws.Hub
	emailer      *notification.Emailer
	jobQueue     *queue.JobQueue
	deadLetter   *DeadLetter
	processedDir string
}

func NewProcessor(
	fileRepo *repo.FileRepo,
	storage *storage.MinioStorage,
	hub *ws.Hub,
	emailer *notification.Emailer,
	jobQueue *queue.JobQueue,
	deadLetter *DeadLetter,
	processedDir string,
) *Processor {
	return &Processor{
		fileRepo:     fileRepo,
		storage:      storage,
		hub:          hub,
		emailer:      emailer,
		jobQueue:     jobQueue,
		deadLetter:   deadLetter,
		processedDir: processedDir,
	}
}

func (p *Processor) Process(ctx context.Context, job domain.Job) {
	logger := log.With().
		Str("file_id", job.FileID.String()).
		Str("file_type", job.FileType).
		Int("retry", job.RetryCount).
		Logger()

	if err := p.doProcess(ctx, job); err != nil {
		logger.Error().Err(err).Msg("processing failed")
		job.RetryCount++

		if job.RetryCount >= maxRetries {
			logger.Warn().Msg("max retries reached, sending to dead letter queue")
			p.deadLetter.Save(ctx, job.FileID, job.UserID, err.Error(), job.RetryCount)
			_ = p.fileRepo.UpdateError(ctx, job.FileID, err.Error())
			p.notifyWS(job.UserID.String(), job.FileID.String(), domain.StatusError, nil, err.Error())
			return
		}

		// Exponential backoff: 2^n seconds (2s, 4s, 8s)
		delay := time.Duration(1<<job.RetryCount) * time.Second
		logger.Info().Dur("delay", delay).Msg("scheduling retry")
		_ = p.fileRepo.IncrementRetry(ctx, job.FileID)

		time.AfterFunc(delay, func() {
			if err := p.jobQueue.Enqueue(job); err != nil {
				logger.Error().Err(err).Msg("failed to re-enqueue job after retry delay")
				p.deadLetter.Save(ctx, job.FileID, job.UserID, "queue full on retry", job.RetryCount)
			}
		})
	}
}

func (p *Processor) doProcess(ctx context.Context, job domain.Job) error {
	// 1. Lock the DB row for this job.
	tx := p.fileRepo.DB().Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin tx: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	file, err := p.fileRepo.FindByIDForUpdate(ctx, tx, job.FileID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("lock file row: %w", err)
	}

	// Guard against duplicate processing (e.g. crash-recovery re-delivery).
	if file.Status == domain.StatusDone || file.Status == domain.StatusError {
		tx.Rollback()
		return nil
	}

	// 2. Mark as processing.
	if err := tx.Model(file).Update("status", domain.StatusProcessing).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("update status processing: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit status processing: %w", err)
	}

	// 3. Process the file.
	if err := os.MkdirAll(p.processedDir, 0o755); err != nil {
		return fmt.Errorf("mkdir processed: %w", err)
	}

	var (
		outputPath  string
		meta        models.FileMeta
		contentType string
	)

	switch job.FileType {
	case domain.FileTypePDF:
		meta, err = ProcessPDF(job.TmpPath)
		if err != nil {
			return fmt.Errorf("pdf processing: %w", err)
		}
		outputPath = job.TmpPath
		contentType = "application/pdf"

	case domain.FileTypeImage:
		outputPath, meta, err = ProcessImage(job.TmpPath, p.processedDir)
		if err != nil {
			return fmt.Errorf("image processing: %w", err)
		}
		contentType = "image/jpeg"
		defer os.Remove(outputPath) // clean up processed tmp file
	}

	// 4. Upload to MinIO. Storage key: {userID}/{fileID}.ext
	storageKey := fmt.Sprintf("%s/%s", job.UserID.String(), job.FileID.String())
	if job.FileType == domain.FileTypePDF {
		storageKey += ".pdf"
	} else {
		storageKey += ".jpg"
	}

	if err := p.storage.PutFile(ctx, storageKey, outputPath, contentType); err != nil {
		return fmt.Errorf("minio upload: %w", err)
	}

	// Clean up tmp file after successful upload.
	_ = os.Remove(job.TmpPath)

	// 5. Update DB: status done + storage path + meta.
	if err := p.fileRepo.UpdateDone(ctx, job.FileID, storageKey, meta); err != nil {
		return fmt.Errorf("update done: %w", err)
	}

	// 6. Push real-time WebSocket notification.
	p.notifyWS(job.UserID.String(), job.FileID.String(), domain.StatusDone, &meta, "")

	// 7. Send email (fire-and-forget goroutine — never blocks the worker).
	go p.emailer.SendProcessed(job.UserEmail, job.OriginalName, job.FileID.String(), domain.StatusDone)

	log.Info().
		Str("file_id", job.FileID.String()).
		Str("storage_key", storageKey).
		Msg("file processed successfully")

	return nil
}

type wsEvent struct {
	Event  string       `json:"event"`
	FileID string       `json:"file_id"`
	Status string       `json:"status"`
	Meta   *models.FileMeta `json:"meta,omitempty"`
	Error  string       `json:"error,omitempty"`
}

func (p *Processor) notifyWS(userID, fileID, status string, meta *models.FileMeta, errMsg string) {
	event := "file.processed"
	if status == domain.StatusError {
		event = "file.error"
	}

	payload, err := json.Marshal(wsEvent{
		Event:  event,
		FileID: fileID,
		Status: status,
		Meta:   meta,
		Error:  errMsg,
	})
	if err != nil {
		return
	}
	p.hub.Send(userID, payload)
}
