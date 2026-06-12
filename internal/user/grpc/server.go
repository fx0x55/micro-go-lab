package grpc

import (
	"context"

	userv1 "github.com/wokoworks/go-server/gen/user/v1"
	"github.com/wokoworks/go-server/internal/user/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserGRPCServer struct {
	userv1.UnimplementedUserServiceServer
	userSvc *service.UserService
}

func NewUserGRPCServer(userSvc *service.UserService) *UserGRPCServer {
	return &UserGRPCServer{userSvc: userSvc}
}

func (s *UserGRPCServer) ValidateUser(ctx context.Context, req *userv1.ValidateUserRequest) (*userv1.ValidateUserResponse, error) {
	user, err := s.userSvc.GetByID(uint(req.UserId))
	if err != nil {
		return &userv1.ValidateUserResponse{Exists: false}, nil
	}
	return &userv1.ValidateUserResponse{Exists: user != nil}, nil
}

func (s *UserGRPCServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	user, err := s.userSvc.GetByID(uint(req.UserId))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "user not found: %v", err)
	}
	return &userv1.GetUserResponse{
		Id:       uint64(user.ID),
		Username: user.Username,
		Email:    user.Email,
	}, nil
}
