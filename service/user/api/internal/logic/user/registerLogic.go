package user

import (
	"context"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrUserExists = ecode.ErrUserExists

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Register 调 user-rpc.CreateUser 创建用户。网关不碰密码哈希、不连库。
func (l *RegisterLogic) Register(req *types.RegisterRequest) (*types.UserResponse, error) {
	resp, err := l.svcCtx.UserCli.CreateUser(l.ctx, &pb.CreateUserRequest{
		Username: req.Username,
		Password: req.Password,
		Email:    req.Email,
	})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return &types.UserResponse{
		ID:       resp.Id,
		Username: resp.Username,
		Email:    resp.Email,
	}, nil
}
