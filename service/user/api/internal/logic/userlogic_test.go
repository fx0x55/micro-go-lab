package logic

import (
	"context"
	"testing"
	"time"

	"github.com/fx0x55/micro-go-lab/common/config"
	"github.com/fx0x55/micro-go-lab/common/model"
	localconfig "github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/mocks"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestLoginLogic_Login(t *testing.T) {
	// 创建gomock控制器
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// 创建mock对象
	mockUserRepo := mocks.NewMockUserRepositoryInterface(ctrl)

	// 准备测试密码
	testPassword := "testpassword123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.DefaultCost)
	require.NoError(t, err, "bcrypt hash should succeed")

	// 定义测试用的用户
	testUser := &model.User{
		ID:        1,
		Username:  "testuser",
		Password:  string(hashedPassword),
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 定义测试用的JWT配置
	jwtConfig := config.JWTConfig{
		Secret:     "test-secret-key-for-testing",
		Expiration: time.Hour * 24,
	}

	// 创建ServiceContext，注入mock
	svcCtx := &svc.ServiceContext{
		Config: localconfig.Config{
			JWT: jwtConfig,
		},
		UserRepo: mockUserRepo,
	}

	tests := []struct {
		name          string
		req           *types.LoginRequest
		mockSetup     func()
		expectedToken bool
		expectedUser  *model.User
		expectedErr   error
	}{
		{
			name: "正常登录成功",
			req: &types.LoginRequest{
				Username: "testuser",
				Password: testPassword,
			},
			mockSetup: func() {
				// Mock: FindByUsername 返回用户
				mockUserRepo.EXPECT().
					FindByUsername(gomock.Any(), "testuser").
					Return(testUser, nil).
					Times(1)
			},
			expectedToken: true,
			expectedUser:  testUser,
			expectedErr:   nil,
		},
		{
			name: "用户不存在",
			req: &types.LoginRequest{
				Username: "nonexistent",
				Password: testPassword,
			},
			mockSetup: func() {
				// Mock: FindByUsername 返回 ErrRecordNotFound
				mockUserRepo.EXPECT().
					FindByUsername(gomock.Any(), "nonexistent").
					Return(nil, gorm.ErrRecordNotFound).
					Times(1)
			},
			expectedToken: false,
			expectedUser:  nil,
			expectedErr:   ErrInvalidCredentials,
		},
		{
			name: "密码错误",
			req: &types.LoginRequest{
				Username: "testuser",
				Password: "wrongpassword",
			},
			mockSetup: func() {
				// Mock: FindByUsername 返回用户（但密码不匹配）
				mockUserRepo.EXPECT().
					FindByUsername(gomock.Any(), "testuser").
					Return(testUser, nil).
					Times(1)
			},
			expectedToken: false,
			expectedUser:  nil,
			expectedErr:   ErrInvalidCredentials,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置mock期望
			tt.mockSetup()

			// 创建Logic实例
			logic := NewLoginLogic(context.Background(), svcCtx)

			// 执行登录
			tokenMap, user, err := logic.Login(tt.req)

			// 验证错误
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr, "错误应该匹配预期")
				assert.Nil(t, tokenMap, "token应该为nil")
				assert.Nil(t, user, "user应该为nil")
				return
			}

			// 验证成功
			require.NoError(t, err, "不应该有错误")
			require.NotNil(t, tokenMap, "token不应该为nil")
			require.NotNil(t, user, "user不应该为nil")

			// 验证token
			if tt.expectedToken {
				tokenStr, ok := tokenMap["token"].(string)
				require.True(t, ok, "token应该是字符串")

				// 解析并验证JWT token
				token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (any, error) {
					return []byte(jwtConfig.Secret), nil
				})
				require.NoError(t, err, "token应该可以解析")
				assert.True(t, token.Valid, "token应该有效")

				// 验证token claims
				claims, ok := token.Claims.(jwt.MapClaims)
				require.True(t, ok, "claims应该是MapClaims")
				assert.InDelta(t, float64(testUser.ID), claims["user_id"], 0.001, "user_id应该匹配")
				assert.Equal(t, testUser.Username, claims["username"], "username应该匹配")
			}

			// 验证返回的用户
			assert.Equal(t, tt.expectedUser.ID, user.ID, "用户ID应该匹配")
			assert.Equal(t, tt.expectedUser.Username, user.Username, "用户名应该匹配")
		})
	}
}
