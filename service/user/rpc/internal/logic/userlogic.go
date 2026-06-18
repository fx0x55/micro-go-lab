package logic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	userv1 "github.com/wokoworks/go-server/service/user/rpc/pb"
	"github.com/wokoworks/go-server/service/user/rpc/internal/svc"
)

const userCacheTTL = 5 * time.Minute
const userCachePrefix = "user:validate:"

type ValidateUserLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewValidateUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ValidateUserLogic {
	return &ValidateUserLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *ValidateUserLogic) ValidateUser(req *userv1.ValidateUserRequest) (*userv1.ValidateUserResponse, error) {
	// 尝试从缓存读取
	if l.svcCtx.Redis != nil {
		key := fmt.Sprintf("%s%d", userCachePrefix, req.UserId)
		val, err := l.svcCtx.Redis.Get(l.ctx, key).Result()
		if err == nil && val == "exists" {
			nameKey := key + ":name"
			name, _ := l.svcCtx.Redis.Get(l.ctx, nameKey).Result()
			return &userv1.ValidateUserResponse{Exists: true, Username: name}, nil
		}
		if err == nil && val == "not_exists" {
			return &userv1.ValidateUserResponse{Exists: false}, nil
		}
	}

	user, err := l.svcCtx.UserRepo.FindByID(uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			l.cacheResult(req.UserId, "not_exists", "")
			return &userv1.ValidateUserResponse{Exists: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "validate user failed: %v", err)
	}
	l.cacheResult(req.UserId, "exists", user.Username)
	return &userv1.ValidateUserResponse{Exists: true, Username: user.Username}, nil
}

func (l *ValidateUserLogic) cacheResult(userID uint64, userStatus, username string) {
	if l.svcCtx.Redis == nil {
		return
	}
	key := fmt.Sprintf("%s%d", userCachePrefix, userID)
	l.svcCtx.Redis.Set(l.ctx, key, userStatus, userCacheTTL)
	if username != "" {
		l.svcCtx.Redis.Set(l.ctx, key+":name", username, userCacheTTL)
	}
}

type GetUserLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserLogic {
	return &GetUserLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetUserLogic) GetUser(req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	user, err := l.svcCtx.UserRepo.FindByID(uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "get user failed: %v", err)
	}
	return &userv1.GetUserResponse{
		Id:       uint64(user.ID),
		Username: user.Username,
		Email:    user.Email,
	}, nil
}
