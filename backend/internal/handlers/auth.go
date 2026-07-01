package handlers

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/Vishal-2029/file-upload-service/internal/services"
	svcinterfaces "github.com/Vishal-2029/file-upload-service/internal/services/interfaces"
)

type AuthHandler struct {
	authSvc svcinterfaces.AuthService
}

func NewAuthHandler(app *fiber.App, authSvc svcinterfaces.AuthService) {
	h := &AuthHandler{authSvc: authSvc}
	auth := app.Group("/auth")
	auth.Post("/register", h.Register)
	auth.Post("/login", h.Login)
}

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register godoc
// POST /auth/register
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req authRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "email and password are required"})
	}
	if len(req.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "password must be at least 8 characters"})
	}

	token, err := h.authSvc.Register(c.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, services.ErrEmailTaken) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "registration failed"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"token": token})
}

// Login godoc
// POST /auth/login
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req authRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	token, err := h.authSvc.Login(c.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, services.ErrInvalidCreds) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "login failed"})
	}

	return c.JSON(fiber.Map{"token": token})
}
