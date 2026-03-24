package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"

	"github.com/Vishal-2029/file-upload-service/internal/models"
	"github.com/Vishal-2029/file-upload-service/internal/services"
)

// mockUserRepo satisfies repo/interfaces.UserRepo for unit tests.
type mockUserRepo struct {
	mock.Mock
}

func (m *mockUserRepo) Create(ctx context.Context, user *models.User) error {
	args := m.Called(ctx, user)
	return args.Error(0)
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func newTestAuthSvc(repo *mockUserRepo) *services.AuthService {
	return services.NewAuthService(repo, "test-secret-32-chars-long-enough", 24)
}

func TestRegister_Success(t *testing.T) {
	repo := &mockUserRepo{}
	repo.On("FindByEmail", mock.Anything, "new@example.com").
		Return(nil, gorm.ErrRecordNotFound)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).
		Return(nil)

	svc := newTestAuthSvc(repo)
	token, err := svc.Register(context.Background(), "new@example.com", "password123")

	assert.NoError(t, err)
	assert.NotEmpty(t, token)
	repo.AssertExpectations(t)
}

func TestRegister_DuplicateEmail(t *testing.T) {
	existing := &models.User{ID: uuid.New(), Email: "dup@example.com"}
	repo := &mockUserRepo{}
	repo.On("FindByEmail", mock.Anything, "dup@example.com").
		Return(existing, nil)

	svc := newTestAuthSvc(repo)
	_, err := svc.Register(context.Background(), "dup@example.com", "password123")

	assert.ErrorIs(t, err, services.ErrEmailTaken)
}

func TestLogin_Success(t *testing.T) {
	// Pre-generate a bcrypt hash for "password123".
	repo := &mockUserRepo{}
	svc := newTestAuthSvc(repo)

	// Register first to get the bcrypt hash.
	repo.On("FindByEmail", mock.Anything, "user@example.com").
		Return(nil, gorm.ErrRecordNotFound).Once()
	var createdUser *models.User
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).
		Run(func(args mock.Arguments) {
			createdUser = args.Get(1).(*models.User)
		}).Return(nil).Once()

	_, err := svc.Register(context.Background(), "user@example.com", "password123")
	assert.NoError(t, err)

	// Now test login.
	repo.On("FindByEmail", mock.Anything, "user@example.com").
		Return(createdUser, nil).Once()

	token, err := svc.Login(context.Background(), "user@example.com", "password123")
	assert.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := &mockUserRepo{}
	svc := newTestAuthSvc(repo)

	// Register.
	repo.On("FindByEmail", mock.Anything, "user@example.com").
		Return(nil, gorm.ErrRecordNotFound).Once()
	var createdUser *models.User
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).
		Run(func(args mock.Arguments) { createdUser = args.Get(1).(*models.User) }).
		Return(nil).Once()
	_, _ = svc.Register(context.Background(), "user@example.com", "correct-password")

	// Login with wrong password.
	repo.On("FindByEmail", mock.Anything, "user@example.com").
		Return(createdUser, nil).Once()

	_, err := svc.Login(context.Background(), "user@example.com", "wrong-password")
	assert.ErrorIs(t, err, services.ErrInvalidCreds)
}

func TestLogin_UserNotFound(t *testing.T) {
	repo := &mockUserRepo{}
	repo.On("FindByEmail", mock.Anything, "ghost@example.com").
		Return(nil, gorm.ErrRecordNotFound)

	svc := newTestAuthSvc(repo)
	_, err := svc.Login(context.Background(), "ghost@example.com", "any")
	assert.ErrorIs(t, err, services.ErrInvalidCreds)
}

func TestValidateToken_Valid(t *testing.T) {
	repo := &mockUserRepo{}
	repo.On("FindByEmail", mock.Anything, "tok@example.com").
		Return(nil, gorm.ErrRecordNotFound)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*models.User")).Return(nil)

	svc := newTestAuthSvc(repo)
	token, err := svc.Register(context.Background(), "tok@example.com", "password123")
	assert.NoError(t, err)

	userID, email, err := svc.ValidateToken(token)
	assert.NoError(t, err)
	assert.NotEmpty(t, userID)
	assert.Equal(t, "tok@example.com", email)
}

func TestValidateToken_Invalid(t *testing.T) {
	repo := &mockUserRepo{}
	svc := newTestAuthSvc(repo)

	_, _, err := svc.ValidateToken("not.a.jwt")
	assert.True(t, errors.Is(err, services.ErrInvalidToken))
}
