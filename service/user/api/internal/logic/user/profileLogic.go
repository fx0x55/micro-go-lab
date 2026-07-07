package user

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
)

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

// GetProfile 从 JWT context 提取 userID，调 user-rpc.GetUser 查询资料。
func (l *ProfileLogic) Profile() (*types.UserResponse, error) {
	userID := middleware.GetUserIDFromContext(l.ctx)

	resp, err := l.svcCtx.UserCli.GetUser(l.ctx, &pb.GetUserRequest{
		UserId: uint64(userID),
	})
	if err != nil {
		if errors.Is(err, ecode.ErrUserNotFound) {
			return nil, ecode.ErrUserNotFound
		}
		return nil, err
	}
	return &types.UserResponse{
		ID:       resp.Id,
		Username: resp.Username,
		Email:    resp.Email,
	}, nil
}
