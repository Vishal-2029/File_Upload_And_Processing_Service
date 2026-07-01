package interfaces

import (
	"context"

	"github.com/Vishal-2029/file-upload-service/internal/models"
)

type UserRepo interface {
	Create(ctx context.Context, user *models.User) error
	FindByEmail(ctx context.Context, email string) (*models.User, error)
}
