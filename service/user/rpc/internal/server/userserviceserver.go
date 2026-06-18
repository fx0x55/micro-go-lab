package server

import (
	"context"

	"github.com/wokoworks/go-server/service/user/rpc/internal/logic"
	"github.com/wokoworks/go-server/service/user/rpc/internal/svc"
	userv1 "github.com/wokoworks/go-server/service/user/rpc/pb"
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
	return l.ValidateUser(req)
}

func (s *UserServiceServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	l := logic.NewGetUserLogic(ctx, s.svcCtx)
	return l.GetUser(req)
}
