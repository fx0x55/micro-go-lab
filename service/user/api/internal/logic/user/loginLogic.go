package user

import (
	"context"
	"time"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/fx0x55/micro-go-lab/service/user/rpc/pb"
	"github.com/golang-jwt/jwt/v5"
	"github.com/zeromicro/go-zero/core/logx"
)

var ErrInvalidCredentials = ecode.ErrInvalidCredentials

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Login 调 user-rpc.Authenticate 校验身份，校验通过后由网关签发 JWT。
func (l *LoginLogic) Login(req *types.LoginRequest) (*types.LoginResponse, error) {
	resp, err := l.svcCtx.UserCli.Authenticate(l.ctx, &pb.AuthenticateRequest{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		return nil, err
	}
	if !resp.Exists {
		return nil, ErrInvalidCredentials
	}

	expire := l.svcCtx.Config.Auth.AccessExpire
	if expire <= 0 {
		expire = int64(24 * time.Hour / time.Second)
	}
	claims := jwt.MapClaims{
		"user_id":  resp.Id,
		"username": resp.Username,
		"exp":      time.Now().Add(time.Duration(expire) * time.Second).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(l.svcCtx.Config.Auth.AccessSecret))
	if err != nil {
		return nil, err
	}
	return &types.LoginResponse{Token: signed}, nil
}
