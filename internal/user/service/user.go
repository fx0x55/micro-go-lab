package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/wokoworks/go-server/internal/config"
	"github.com/wokoworks/go-server/internal/user/model"
	"github.com/wokoworks/go-server/internal/user/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrUserExists      = errors.New("username or email already exists")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

type UserService struct {
	userRepo *repository.UserRepository
	jwtCfg   config.JWTConfig
}

func NewUserService(userRepo *repository.UserRepository, jwtCfg config.JWTConfig) *UserService {
	return &UserService{userRepo: userRepo, jwtCfg: jwtCfg}
}

type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=64"`
	Password string `json:"password" validate:"required,min=6,max=128"`
	Email    string `json:"email" validate:"required,email"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	Token string      `json:"token"`
	User  *model.User `json:"user"`
}

func (s *UserService) Register(req *RegisterRequest) (*model.User, error) {
	if _, err := s.userRepo.FindByUsername(req.Username); err == nil {
		return nil, ErrUserExists
	}
	if _, err := s.userRepo.FindByEmail(req.Email); err == nil {
		return nil, ErrUserExists
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: req.Username,
		Password: string(hashed),
		Email:    req.Email,
	}

	if err := s.userRepo.Create(user); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, ErrUserExists
		}
		return nil, err
	}
	return user, nil
}

func (s *UserService) Login(req *LoginRequest) (*LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{Token: token, User: user}, nil
}

func (s *UserService) GetByID(id uint) (*model.User, error) {
	return s.userRepo.FindByID(id)
}

func (s *UserService) generateToken(user *model.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(s.jwtCfg.Expiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtCfg.Secret))
}
