package logic

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type AuthenticateLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewAuthenticateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AuthenticateLogic {
	return &AuthenticateLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// Authenticate 校验用户名+密码。用户不存在或密码错误统一返回 exists=false，
// 不区分两种失败以防用户名枚举。成功时返回身份（id + username），密码哈希永不出域。
func (l *AuthenticateLogic) Authenticate(in *pb.AuthenticateRequest) (*pb.AuthenticateResponse, error) {
	user, err := l.svcCtx.UserRepo.FindByUsername(l.ctx, in.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &pb.AuthenticateResponse{Exists: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "query user failed: %v", err)
	}

	// 密码不匹配：与「用户不存在」返回相同语义，避免枚举。
	//nolint:nilerr // intentional: hide bcrypt mismatch from caller to prevent enumeration
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(in.Password)); err != nil {
		return &pb.AuthenticateResponse{
			Exists: false,
		}, nil
	}

	return &pb.AuthenticateResponse{
		Exists:   true,
		Id:       uint64(user.ID),
		Username: user.Username,
	}, nil
}
