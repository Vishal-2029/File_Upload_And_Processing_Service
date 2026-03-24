package domain

import "github.com/google/uuid"

// Status constants for file processing lifecycle.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusDone       = "done"
	StatusError      = "error"
)

// FileType constants for supported upload types.
const (
	FileTypePDF   = "pdf"
	FileTypeImage = "image"
)

// AllowedMIMETypes maps MIME type to FileType.
var AllowedMIMETypes = map[string]string{
	"application/pdf": FileTypePDF,
	"image/jpeg":      FileTypeImage,
	"image/png":       FileTypeImage,
	"image/gif":       FileTypeImage,
	"image/webp":      FileTypeImage,
}

// Job is the unit of work enqueued for async processing.
type Job struct {
	FileID       uuid.UUID
	UserID       uuid.UUID
	UserEmail    string
	TmpPath      string
	FileType     string
	OriginalName string
	RetryCount   int
}
