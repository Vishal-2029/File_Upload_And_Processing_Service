package services

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/Vishal-2029/file-upload-service/internal/models"
	repointerfacer "github.com/Vishal-2029/file-upload-service/internal/repo/interfaces"
)

var (
	ErrEmailTaken       = errors.New("email already registered")
	ErrInvalidCreds     = errors.New("invalid email or password")
	ErrInvalidToken     = errors.New("invalid or expired token")
)

type AuthService struct {
	userRepo    repointerfacer.UserRepo
	jwtSecret   []byte
	expiryHours int
}

func NewAuthService(userRepo repointerfacer.UserRepo, jwtSecret string, expiryHours int) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		jwtSecret:   []byte(jwtSecret),
		expiryHours: expiryHours,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) (string, error) {
	existing, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if existing != nil {
		return "", ErrEmailTaken
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	user := &models.User{
		ID:       uuid.New(),
		Email:    email,
		Password: string(hash),
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return "", err
	}

	return s.signToken(user.ID.String(), user.Email)
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrInvalidCreds
		}
		return "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", ErrInvalidCreds
	}

	return s.signToken(user.ID.String(), user.Email)
}

func (s *AuthService) ValidateToken(tokenStr string) (string, string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", "", ErrInvalidToken
	}

	claims, ok := token.Claims.(*jwt.MapClaims)
	if !ok {
		return "", "", ErrInvalidToken
	}

	userID, _ := (*claims)["sub"].(string)
	email, _ := (*claims)["email"].(string)
	return userID, email, nil
}

func (s *AuthService) signToken(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"exp":   time.Now().Add(time.Duration(s.expiryHours) * time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}
