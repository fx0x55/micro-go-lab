package logic

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/fx0x55/micro-go-lab/common/xcache"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type ValidateUserLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// userSummary 是 ValidateUser 的缓存载荷。Cache 在 ServiceContext 中以
// "user:validate:" 前缀构造，此处 key 仅传用户 ID。
type userSummary struct {
	Exists   bool   `json:"exists"`
	Username string `json:"username,omitempty"`
}

func NewValidateUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ValidateUserLogic {
	return &ValidateUserLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// ValidateUser 验证用户是否存在
func (l *ValidateUserLogic) ValidateUser(in *pb.ValidateUserRequest) (*pb.ValidateUserResponse, error) {
	summary, err := xcache.GetOrLoad(l.ctx, l.svcCtx.Cache, strconv.FormatUint(in.UserId, 10),
		func(s userSummary) ([]byte, error) { return json.Marshal(s) },
		func(b []byte) (userSummary, error) {
			var s userSummary
			if uerr := json.Unmarshal(b, &s); uerr != nil {
				return userSummary{}, uerr
			}
			return s, nil
		},
		func(ctx context.Context) (userSummary, error) {
			user, err := l.svcCtx.UserRepo.FindByID(ctx, uint(in.UserId))
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
			return &pb.ValidateUserResponse{Exists: false}, nil
		}
		// 已是带 codes.Internal 的 gRPC status 错误，原样返回。
		return nil, err
	}
	return &pb.ValidateUserResponse{Exists: summary.Exists, Username: summary.Username}, nil
}
