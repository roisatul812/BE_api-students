package handler

import (
	"errors"

	"api-students/app/model"
	"api-students/app/service"
	"api-students/helper"

	"github.com/gofiber/fiber/v2"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req model.RegisterRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "request body tidak valid",
		})
	}

	user, err := h.authService.Register(c.Context(), req)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"message": "validasi gagal",
				"errors":  validationErr.Errors,
			})
		}

		var conflictErr *service.ConflictError
		if errors.As(err, &conflictErr) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"message": conflictErr.Message,
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal melakukan registrasi",
		})
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req model.LoginRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "request body tidak valid",
		})
	}

	tokens, err := h.authService.Login(c.Context(), req)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": "validasi gagal",
				"errors":  validationErr.Errors,
			})
		}

		if errors.Is(err, service.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "username atau password salah",
			})
		}

		if errors.Is(err, service.ErrInactiveUser) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "akun tidak aktif",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal melakukan login",
		})
	}

	return c.JSON(tokens)
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req model.RefreshRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "request body tidak valid",
		})
	}

	tokens, err := h.authService.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRefreshToken) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "refresh token tidak valid",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal melakukan refresh token",
		})
	}

	return c.JSON(tokens)
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req model.RefreshRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "request body tidak valid",
		})
	}

	if err := h.authService.Logout(c.Context(), req.RefreshToken); err != nil {
		if errors.Is(err, service.ErrInvalidRefreshToken) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "refresh token tidak valid",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal melakukan logout",
		})
	}

	return c.Status(fiber.StatusNoContent).Send(nil)
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	authUser, ok := helper.CurrentUser(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "user belum terautentikasi",
		})
	}

	user, err := h.authService.Me(c.Context(), authUser.UserID)
	if err != nil {
		var notFoundErr *service.NotFoundError
		if errors.As(err, &notFoundErr) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": notFoundErr.Message,
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "gagal mengambil data user",
		})
	}

	return c.JSON(user)
}
