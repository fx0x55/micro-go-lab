package logic

import (
	"context"
	"errors"
	"time"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// 保留向后兼容的别名（供 handler 使用 logic.ErrXxx）。
var (
	ErrUserExists         = ecode.ErrUserExists
	ErrInvalidCredentials = ecode.ErrInvalidCredentials
)

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
	result, err := l.svcCtx.UserCli.CreateUser(l.ctx, req.Username, req.Password, req.Email)
	if err != nil {
		// gRPC AlreadyExists → 业务冲突。
		if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return &types.UserResponse{
		ID:       result.ID,
		Username: result.Username,
		Email:    result.Email,
	}, nil
}

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
// 密码比对在 user-rpc 内部完成，哈希不出域。
func (l *LoginLogic) Login(req *types.LoginRequest) (*types.LoginResponse, error) {
	auth, err := l.svcCtx.UserCli.Authenticate(l.ctx, req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	if !auth.Exists {
		// 不区分「用户不存在」与「密码错误」，防枚举。
		return nil, ErrInvalidCredentials
	}

	token, err := generateToken(auth.ID, auth.Username, l.svcCtx.Config.JWT)
	if err != nil {
		return nil, err
	}
	return &types.LoginResponse{Token: token}, nil
}

func generateToken(userID uint64, username string, jwtCfg config.JWTConfig) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(jwtCfg.Expiration).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtCfg.Secret))
}

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

// GetProfile 调 user-rpc.GetUser 查询资料。
func (l *ProfileLogic) GetProfile(userID uint) (*types.UserResponse, error) {
	resp, err := l.svcCtx.UserCli.GetUser(l.ctx, userID)
	if err != nil {
		if errors.Is(err, ecode.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	return &types.UserResponse{
		ID:       resp.Id,
		Username: resp.Username,
		Email:    resp.Email,
	}, nil
}
