package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	svcinterfaces "github.com/Vishal-2029/file-upload-service/internal/services/interfaces"
)

// JWTMiddleware extracts and validates Bearer tokens.
// On success it sets c.Locals("userID") and c.Locals("userEmail").
func JWTMiddleware(authSvc svcinterfaces.AuthService) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authorization header"})
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid authorization format"})
		}

		userID, email, err := authSvc.ValidateToken(parts[1])
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired token"})
		}

		c.Locals("userID", userID)
		c.Locals("userEmail", email)
		return c.Next()
	}
}
