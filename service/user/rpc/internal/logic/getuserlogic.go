package logic

import (
	"context"

	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

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

// GetUser 获取用户信息
func (l *GetUserLogic) GetUser(in *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	user, err := l.svcCtx.UserRepo.FindByID(l.ctx, uint(in.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "get user failed: %v", err)
	}
	return &pb.GetUserResponse{
		Id:       uint64(user.ID),
		Username: user.Username,
		Email:    user.Email,
	}, nil
}
