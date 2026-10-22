package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	"api-students/app/handler"
	"api-students/app/repository"
	"api-students/app/service"
	"api-students/config"
	"api-students/database"
	"api-students/helper"
	"api-students/middleware"
	"api-students/route"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("file .env tidak ditemukan, menggunakan environment variable")
	}

	authConfig := config.LoadAuthConfig()

	if authConfig.JWTSecret == "" {
		log.Fatal("JWT_SECRET wajib diisi")
	}

	ctx := context.Background()

	// Logger terpusat
	slogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// PostgreSQL connection
	db, err := database.Connect(ctx)
	if err != nil {
		slogger.Error("gagal terhubung ke PostgreSQL", slog.String("error", err.Error()))
		log.Fatal(err)
	}
	defer db.Close()

	// MongoDB v2 connection
	mongoCfg := database.LoadMongoConfig()
	mongoClient, err := database.ConnectMongo(ctx, mongoCfg)
	if err != nil {
		slogger.Error("gagal terhubung ke MongoDB", slog.String("error", err.Error()))
		log.Fatalf("MongoDB wajib tersedia untuk Modul 10: %v", err)
	}
	defer func() {
		if err := database.DisconnectMongo(mongoClient, 5*time.Second); err != nil {
			slogger.Warn("gagal menutup koneksi MongoDB secara anggun", slog.String("error", err.Error()))
		}
	}()

	mongoDB := database.GetMongoDatabase(mongoClient, mongoCfg.DatabaseName)
	reviewRepository := repository.NewReviewRepository(mongoDB)
	if err := reviewRepository.EnsureIndexes(ctx); err != nil {
		slogger.Error("gagal membuat index MongoDB", slog.String("error", err.Error()))
		log.Fatalf("gagal membuat index MongoDB: %v", err)
	}

	portfolioRepository := repository.NewPortfolioRepository(mongoDB)
	if err := portfolioRepository.EnsureIndexes(ctx); err != nil {
		slogger.Error("gagal membuat index portfolio MongoDB", slog.String("error", err.Error()))
		log.Fatalf("gagal membuat index portfolio MongoDB: %v", err)
	}

	// Permission dependencies
	roleRepository := repository.NewRoleRepository(db)

	permissions, err := roleRepository.LoadPermissions(ctx)
	if err != nil {
		log.Fatalf("gagal memuat permission: %v", err)
	}

	permissionSet := helper.NewPermissionSet(permissions)

	// Student dependencies
	studentRepository := repository.NewPostgresStudentRepository(db)

	studentService := service.NewStudentService(
		studentRepository,
		permissionSet,
	)

	studentHandler := handler.NewStudentHandler(studentService)

	// Prestasi dependencies
	prestasiRepository := repository.NewPrestasiRepository(db)

	prestasiService := service.NewPrestasiService(
		prestasiRepository,
	)

	prestasiHandler := handler.NewPrestasiHandler(prestasiService)

	// Authentication dependencies
	userRepository := repository.NewUserRepository(db)
	tokenRepository := repository.NewTokenRepository(db)

	jwtManager := helper.NewJWTManager(
		authConfig.JWTSecret,
		authConfig.JWTIssuer,
		authConfig.JWTAccessTTL,
	)

	authService := service.NewAuthService(
		userRepository,
		tokenRepository,
		jwtManager,
		authConfig.JWTRefreshTTL,
	)

	authHandler := handler.NewAuthHandler(authService)

	// Fiber
	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(slogger),
	})

	app.Use(requestid.New())
	app.Use(middleware.RequestLogger(slogger))
	app.Use(logger.New())

	// Health check memeriksa ketersediaan PostgreSQL dan MongoDB
	app.Get("/health", func(c *fiber.Ctx) error {
		hCtx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		if err := db.Ping(hCtx); err != nil {
			return helper.ServiceUnavailable("PostgreSQL tidak dapat dihubungi")
		}

		if err := mongoClient.Ping(hCtx, nil); err != nil {
			return helper.ServiceUnavailable("MongoDB tidak dapat dihubungi")
		}

		return helper.Success(c, fiber.StatusOK, "server, PostgreSQL, dan MongoDB berjalan", nil)
	})

	// Course & KRS dependencies
	courseRepository := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)
	courseService := service.NewCourseService(courseRepository, txManager)

	// Supervisor & Tugas Akhir dependencies
	supervisorRepository := repository.NewSupervisorRepository(db)
	supervisorService := service.NewSupervisorService(supervisorRepository, txManager)

	// Review dependencies
	reviewService := service.NewReviewService(reviewRepository, courseRepository, permissionSet)

	// Portfolio dependencies
	portfolioService := service.NewPortfolioService(portfolioRepository, studentRepository, permissionSet)

	// Routes
	route.AuthRoutes(
		app,
		authHandler,
		jwtManager,
	)

	route.StudentRoutes(
		app,
		studentHandler,
		prestasiHandler,
		jwtManager,
		permissionSet,
	)

	route.CourseRoutes(
		app,
		courseService,
		jwtManager,
		permissionSet,
	)

	route.SupervisorRoutes(
		app,
		supervisorService,
		jwtManager,
		permissionSet,
	)

	route.ReviewRoutes(
		app,
		reviewService,
		jwtManager,
	)

	route.PortfolioRoutes(
		app,
		portfolioService,
		jwtManager,
	)

	// Route tidak ditemukan (404) akan diteruskan ke ErrorHandler
	app.Use(func(c *fiber.Ctx) error {
		return helper.NotFound("endpoint tidak ditemukan")
	})

	log.Println("server berjalan di http://localhost:3000")

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Fatal(app.Listen(":" + port))
}
