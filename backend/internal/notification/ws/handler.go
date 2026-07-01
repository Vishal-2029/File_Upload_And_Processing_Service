package ws

import (
	"strings"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// Handler returns a Fiber WebSocket handler that authenticates via JWT query param.
// Clients connect as: ws://host/ws?token=<jwt>
func Handler(hub *Hub, jwtSecret string) fiber.Handler {
	secret := []byte(jwtSecret)

	return websocket.New(func(c *websocket.Conn) {
		tokenStr := c.Query("token")
		if tokenStr == "" {
			_ = c.Close()
			return
		}

		userID := validateWSToken(tokenStr, secret)
		if userID == "" {
			_ = c.Close()
			return
		}

		client := NewClient(userID, c, hub)
		hub.Register(client)

		go client.WritePump()
		client.ReadPump() // blocks until disconnect
	})
}

// validateWSToken parses and validates a JWT, returning the subject (userID) or "".
func validateWSToken(tokenStr string, secret []byte) string {
	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenStr, &jwt.MapClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return ""
	}

	claims, ok := token.Claims.(*jwt.MapClaims)
	if !ok {
		return ""
	}

	sub, _ := (*claims)["sub"].(string)
	return sub
}
