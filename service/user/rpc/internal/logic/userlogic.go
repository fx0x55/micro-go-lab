package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/wokoworks/go-server/common/xcache"
	"github.com/wokoworks/go-server/service/user/rpc/internal/svc"
	userv1 "github.com/wokoworks/go-server/service/user/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// userSummary 是 ValidateUser 的缓存载荷。Cache 在 ServiceContext 中以
// "user:validate:" 前缀构造，此处 key 仅传用户 ID。
type userSummary struct {
	Exists   bool   `json:"exists"`
	Username string `json:"username,omitempty"`
}

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
	summary, err := xcache.GetOrLoad(l.ctx, l.svcCtx.Cache, strconv.FormatUint(req.UserId, 10),
		func(s userSummary) ([]byte, error) { return json.Marshal(s) },
		func(b []byte) (userSummary, error) {
			var s userSummary
			if uerr := json.Unmarshal(b, &s); uerr != nil {
				return userSummary{}, uerr
			}
			return s, nil
		},
		func(ctx context.Context) (userSummary, error) {
			user, err := l.svcCtx.UserRepo.FindByID(ctx, uint(req.UserId))
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// 用户不存在 → 负缓存（短 TTL，见 CacheConfig.NegativeTTL）。
					return userSummary{}, xcache.ErrMiss
				}
				return userSummary{}, status.Errorf(codes.Internal, "validate user failed: %v", err)
			}
			return userSummary{Exists: true, Username: user.Username}, nil
		})
	if err != nil {
		if errors.Is(err, xcache.ErrMiss) {
			return &userv1.ValidateUserResponse{Exists: false}, nil
		}
		// 已是带 codes.Internal 的 gRPC status 错误，原样返回。
		return nil, err
	}
	return &userv1.ValidateUserResponse{Exists: summary.Exists, Username: summary.Username}, nil
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
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, uint(req.UserId))
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
