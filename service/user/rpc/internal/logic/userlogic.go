package logic

import (
	"context"
	"errors"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	userv1 "github.com/wokoworks/go-server/service/user/rpc/pb"
	"github.com/wokoworks/go-server/service/user/rpc/internal/svc"
)

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
	user, err := l.svcCtx.UserRepo.FindByID(uint(req.UserId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &userv1.ValidateUserResponse{Exists: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "validate user failed: %v", err)
	}
	return &userv1.ValidateUserResponse{Exists: true, Username: user.Username}, nil
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
