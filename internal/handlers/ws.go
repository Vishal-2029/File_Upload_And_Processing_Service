package handlers

import (
	"github.com/gofiber/fiber/v2"
	ws "github.com/Vishal-2029/file-upload-service/internal/notification/ws"
)

// RegisterWSHandler mounts the WebSocket upgrade endpoint.
func RegisterWSHandler(app *fiber.App, hub *ws.Hub, jwtSecret string) {
	app.Get("/ws", ws.Handler(hub, jwtSecret))
}
