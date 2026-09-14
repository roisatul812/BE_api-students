package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"
)

var ErrInvalidCredentials = errors.New("username atau password salah")
var ErrInactiveUser = errors.New("akun tidak aktif")
var ErrInvalidRefreshToken = errors.New("refresh token tidak valid")

type AuthService struct {
	users      repository.UserRepository
	tokens     repository.TokenRepository
	jwt        *helper.JWTManager
	refreshTTL time.Duration
}

func NewAuthService(
	users repository.UserRepository,
	tokens repository.TokenRepository,
	jwtManager *helper.JWTManager,
	refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		users:      users,
		tokens:     tokens,
		jwt:        jwtManager,
		refreshTTL: refreshTTL,
	}
}

func (s *AuthService) Register(
	ctx context.Context,
	req model.RegisterRequest,
) (*model.User, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)

	if errs := ValidateRegister(req); len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}

	existing, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, &ConflictError{Message: "username sudah digunakan"}
	}

	existingEmail, err := s.users.FindByEmail(ctx, req.Email)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if existingEmail != nil {
		return nil, &ConflictError{Message: "email sudah digunakan"}
	}

	hashedPassword, err := helper.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username:  req.Username,
		Email:     req.Email,
		Password:  hashedPassword,
		IsActive:  true,
		Role:      "user",
		CreatedAt: time.Now(),
	}

	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *AuthService) Login(
	ctx context.Context,
	req model.LoginRequest,
) (*model.TokenPair, error) {
	req.Username = strings.TrimSpace(req.Username)

	if errs := ValidateLogin(req); len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}

	user, err := s.users.FindByUsername(ctx, req.Username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.IsActive {
		return nil, ErrInactiveUser
	}

	if err := helper.CheckPassword(req.Password, user.Password); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, err := s.jwt.GenerateAccess(*user)
	if err != nil {
		return nil, err
	}

	refreshToken, err := helper.RandomToken(32)
	if err != nil {
		return nil, err
	}

	refreshTokenData := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.SHA256Hex(refreshToken),
		ExpiresAt: time.Now().Add(s.refreshTTL),
		CreatedAt: time.Now(),
	}

	if err := s.tokens.Save(ctx, refreshTokenData); err != nil {
		return nil, err
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwt.AccessTTL().Seconds()),
	}, nil
}

func (s *AuthService) Refresh(
	ctx context.Context,
	refreshToken string,
) (*model.TokenPair, error) {
	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		return nil, ErrInvalidRefreshToken
	}

	tokenHash := helper.SHA256Hex(refreshToken)

	storedToken, err := s.tokens.FindActive(ctx, tokenHash)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	user, err := s.users.FindByID(ctx, storedToken.UserID)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	if !user.IsActive {
		return nil, ErrInvalidRefreshToken
	}

	if err := s.tokens.Revoke(ctx, tokenHash); err != nil {
		return nil, err
	}

	accessToken, err := s.jwt.GenerateAccess(*user)
	if err != nil {
		return nil, err
	}

	newRefreshToken, err := helper.RandomToken(32)
	if err != nil {
		return nil, err
	}

	newRefreshTokenData := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: helper.SHA256Hex(newRefreshToken),
		ExpiresAt: time.Now().Add(s.refreshTTL),
		CreatedAt: time.Now(),
	}

	if err := s.tokens.Save(ctx, newRefreshTokenData); err != nil {
		return nil, err
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.jwt.AccessTTL().Seconds()),
	}, nil
}

func (s *AuthService) Logout(
	ctx context.Context,
	refreshToken string,
) error {
	refreshToken = strings.TrimSpace(refreshToken)

	if refreshToken == "" {
		return ErrInvalidRefreshToken
	}

	return s.tokens.Revoke(
		ctx,
		helper.SHA256Hex(refreshToken),
	)
}

func (s *AuthService) Me(
	ctx context.Context,
	userID int,
) (*model.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, &NotFoundError{
				Message: "user tidak ditemukan",
			}
		}

		return nil, err
	}

	return user, nil
}
