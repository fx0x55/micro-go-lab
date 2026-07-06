package types

type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=64"`
	Password string `json:"password" validate:"required,min=6,max=128"`
	Email    string `json:"email"    validate:"required,email"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// 网关响应类型：仅暴露契约字段，不含密码等内部实现细节。

type UserResponse struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
}

type LoginResponse struct {
	Token string `json:"token"`
}
