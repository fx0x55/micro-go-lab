package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/common/model"
	"github.com/fx0x55/micro-go-lab/common/xevent"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 保留向后兼容的别名（供 handler 使用 logic.ErrXxx）
var (
	ErrUserExists         = ecode.ErrUserExists
	ErrInvalidCredentials = ecode.ErrInvalidCredentials
	ErrUserNotFound       = ecode.ErrUserNotFound
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RegisterLogic) Register(req *types.RegisterRequest) (*model.User, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: req.Username,
		Password: string(hashed),
		Email:    req.Email,
	}

	err = l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
		if err := l.svcCtx.UserRepo.Create(tx, user); err != nil {
			mysqlErr := &mysql.MySQLError{}
			if errors.As(err, &mysqlErr) {
				return ErrUserExists
			}
			return err
		}

		payload, err := json.Marshal(xevent.Envelope{
			Event:      xevent.UserRegistered,
			Version:    1,
			OccurredAt: time.Now(),
			Data: xevent.UserRegisteredData{
				UserID:   user.ID,
				Username: user.Username,
			},
		})
		if err != nil {
			return err
		}
		outboxEvent := &xevent.OutboxEvent{
			EventID:   uuid.New().String(),
			Topic:     xevent.TopicUserEvents,
			EventKey:  strconv.FormatUint(uint64(user.ID), 10),
			EventType: string(xevent.UserRegistered),
			Version:   1,
			Payload:   string(payload),
			Status:    xevent.OutboxStatusPending,
		}
		return l.svcCtx.OutboxRepo.Insert(tx, outboxEvent)
	})
	if err != nil {
		return nil, err
	}

	return user, nil
}

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginRequest) (map[string]any, *model.User, error) {
	user, err := l.svcCtx.UserRepo.FindByUsername(l.ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrInvalidCredentials
		}
		return nil, nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	token, err := generateToken(user, l.svcCtx.Config.JWT)
	if err != nil {
		return nil, nil, err
	}

	return map[string]any{"token": token}, user, nil
}

func generateToken(user *model.User, jwtCfg config.JWTConfig) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"exp":      time.Now().Add(jwtCfg.Expiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtCfg.Secret))
}

type ProfileLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewProfileLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ProfileLogic {
	return &ProfileLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ProfileLogic) GetProfile(userID uint) (*model.User, error) {
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}
