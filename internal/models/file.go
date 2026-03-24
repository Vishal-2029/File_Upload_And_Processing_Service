package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FileMeta holds processing results stored as JSONB.
type FileMeta struct {
	// PDF fields
	PageCount int `json:"page_count,omitempty"`
	WordCount int `json:"word_count,omitempty"`

	// Image fields
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Format string `json:"format,omitempty"`

	// Error info
	Error string `json:"error,omitempty"`
}

// Value implements driver.Valuer for GORM JSONB storage.
func (m FileMeta) Value() (driver.Value, error) {
	return json.Marshal(m)
}

// Scan implements sql.Scanner for GORM JSONB retrieval.
func (m *FileMeta) Scan(value interface{}) error {
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported type for FileMeta: %T", value)
	}
	return json.Unmarshal(b, m)
}

type File struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID       uuid.UUID `gorm:"type:uuid;not null;index"`
	Status       string    `gorm:"not null;default:pending"`
	FileType     string    `gorm:"not null"`
	OriginalName string    `gorm:"not null"`
	StoragePath  string
	Meta         FileMeta  `gorm:"type:jsonb;not null;default:'{}'"`
	RetryCount   int       `gorm:"not null;default:0"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
