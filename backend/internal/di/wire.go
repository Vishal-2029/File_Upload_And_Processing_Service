package di

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	fiberlog "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Vishal-2029/file-upload-service/config"
	"github.com/Vishal-2029/file-upload-service/internal/db"
	"github.com/Vishal-2029/file-upload-service/internal/handlers"
	"github.com/Vishal-2029/file-upload-service/internal/middleware"
	"github.com/Vishal-2029/file-upload-service/internal/models"
	"github.com/Vishal-2029/file-upload-service/internal/notification"
	ws "github.com/Vishal-2029/file-upload-service/internal/notification/ws"
	"github.com/Vishal-2029/file-upload-service/internal/queue"
	"github.com/Vishal-2029/file-upload-service/internal/repo"
	"github.com/Vishal-2029/file-upload-service/internal/services"
	"github.com/Vishal-2029/file-upload-service/internal/storage"
	"github.com/Vishal-2029/file-upload-service/internal/worker"
)

func Init() {
	// ── Logging ──────────────────────────────────────────────────────────────
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	log.Info().Msg("starting file upload & processing service")

	// ── Config ───────────────────────────────────────────────────────────────
	cfg := config.Load()

	// ── Database ─────────────────────────────────────────────────────────────
	database := db.Connect(cfg.PostgresDSN)

	// AutoMigrate for development convenience.
	if err := database.AutoMigrate(&models.User{}, &models.File{}); err != nil {
		log.Fatal().Err(err).Msg("automigrate failed")
	}
	if err := database.AutoMigrate(&worker.DeadLetterJob{}); err != nil {
		log.Fatal().Err(err).Msg("automigrate dead_letter failed")
	}

	// ── Storage ───────────────────────────────────────────────────────────────
	minioStorage := storage.NewMinioStorage(
		cfg.MinioEndpoint,
		cfg.MinioAccessKey,
		cfg.MinioSecretKey,
		cfg.MinioBucket,
		cfg.MinioUseSSL,
	)

	// ── Repositories ─────────────────────────────────────────────────────────
	userRepo := repo.NewUserRepo(database)
	fileRepo := repo.NewFileRepo(database)

	// ── Services ─────────────────────────────────────────────────────────────
	authSvc := services.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours)
	fileSvc := services.NewFileService(fileRepo, minioStorage)

	// ── Job Queue ─────────────────────────────────────────────────────────────
	jobQueue := queue.NewJobQueue(cfg.JobQueueSize)

	// ── WebSocket Hub ─────────────────────────────────────────────────────────
	hub := ws.NewHub()
	go hub.Run()

	// ── Notifications ─────────────────────────────────────────────────────────
	emailer := notification.NewEmailer(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom)

	// ── Worker Pool ────────────────────────────────────────────────────────────
	deadLetter := worker.NewDeadLetter(database)
	processor := worker.NewProcessor(
		fileRepo,
		minioStorage,
		hub,
		emailer,
		jobQueue,
		deadLetter,
		cfg.ProcessedDir,
	)

	ctx, cancel := context.WithCancel(context.Background())
	pool := worker.NewPool(jobQueue.Channel(), cfg.WorkerCount, processor)
	pool.Start(ctx)

	// ── HTTP App ──────────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		BodyLimit:    int(cfg.MaxFileSizeMB * 1024 * 1024),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	app.Use(recover.New())
	app.Use(fiberlog.New(fiberlog.Config{
		Format: "${time} ${method} ${path} ${status} ${latency}\n",
	}))

	// Public routes.
	handlers.NewAuthHandler(app, authSvc)

	// Protected routes: JWT required everywhere below.
	jwtMW := middleware.JWTMiddleware(authSvc)
	rateMW := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst).Middleware()

	protected := app.Group("/", jwtMW)

	// /upload additionally enforces per-user rate limiting.
	uploadGroup := app.Group("/", jwtMW, rateMW)
	handlers.NewUploadHandler(uploadGroup, fileSvc, jobQueue, cfg.TmpDir, cfg.MaxFileSizeMB)

	handlers.NewFileHandler(protected, fileSvc)

	// WebSocket (JWT via query param, not Bearer header).
	handlers.RegisterWSHandler(app, hub, cfg.JWTSecret)

	// Health endpoint.
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ── Graceful Shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info().Msg("shutdown signal received")

		jobQueue.Close()
		cancel()
		pool.Wait()

		if err := app.ShutdownWithTimeout(15 * time.Second); err != nil {
			log.Error().Err(err).Msg("http shutdown error")
		}
		hub.Stop()
		log.Info().Msg("shutdown complete")
	}()

	log.Info().Str("addr", cfg.HTTPAddr).Msg("server listening")
	if err := app.Listen(cfg.HTTPAddr); err != nil {
		log.Fatal().Err(err).Msg("server error")
	}
}
