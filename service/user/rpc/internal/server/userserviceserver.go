package server

import (
	"context"

	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/logic"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/svc"
	userv1 "github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
)

type UserServiceServer struct {
	svcCtx *svc.ServiceContext
	userv1.UnimplementedUserServiceServer
}

func NewUserServiceServer(svcCtx *svc.ServiceContext) *UserServiceServer {
	return &UserServiceServer{svcCtx: svcCtx}
}

func (s *UserServiceServer) ValidateUser(
	ctx context.Context,
	req *userv1.ValidateUserRequest,
) (*userv1.ValidateUserResponse, error) {
	l := logic.NewValidateUserLogic(ctx, s.svcCtx)
	logx.Debugf("ValidateUser req: %v", req)
	return l.ValidateUser(req)
}

func (s *UserServiceServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	l := logic.NewGetUserLogic(ctx, s.svcCtx)
	logx.Debugf("GetUser req: %v", req)
	return l.GetUser(req)
}
