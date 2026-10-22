package main

import (
	"context"
	"log"
	"log/slog"
	"os"

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

	db, err := database.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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

	// Logger untuk error handler
	slogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Fiber
	app := fiber.New(fiber.Config{
		ErrorHandler: config.NewErrorHandler(slogger),
	})

	app.Use(requestid.New())
	app.Use(middleware.RequestLogger(slogger))
	app.Use(logger.New())

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	// Course & KRS dependencies
	courseRepository := repository.NewCourseRepository(db)
	txManager := repository.NewTxManager(db)
	courseService := service.NewCourseService(courseRepository, txManager)

	// Supervisor & Tugas Akhir dependencies
	supervisorRepository := repository.NewSupervisorRepository(db)
	supervisorService := service.NewSupervisorService(supervisorRepository, txManager)

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
