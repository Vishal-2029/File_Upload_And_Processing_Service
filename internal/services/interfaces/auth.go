package interfaces

import "context"

type AuthService interface {
	Register(ctx context.Context, email, password string) (token string, err error)
	Login(ctx context.Context, email, password string) (token string, err error)
	ValidateToken(tokenStr string) (userID string, email string, err error)
}
