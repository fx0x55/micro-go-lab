package ecode

// 统一业务错误码，所有服务共用。
// 格式: 服务前缀(2位) + 模块(2位) + 序号(2位)

type Code struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Code) Error() string { return c.Message }

func New(code int, msg string) *Code { return &Code{Code: code, Message: msg} }

// 通用错误码 1xxxxx
var (
	OK              = &Code{0, "ok"}
	ErrParam        = New(100001, "invalid parameter")
	ErrUnauthorized = New(100002, "unauthorized")
	ErrNotFound     = New(100003, "resource not found")
	ErrInternal     = New(100004, "internal server error")
	ErrRateLimit    = New(100005, "rate limit exceeded")
)

// 用户模块 2xxxxx
var (
	ErrUserExists         = New(200001, "username or email already exists")
	ErrInvalidCredentials = New(200002, "invalid username or password")
	ErrUserNotFound       = New(200003, "user not found")
)

// 待办模块 3xxxxx
var (
	ErrTodoNotFound = New(300001, "todo not found")
)

// 订单模块 4xxxxx
var (
	ErrOrderNotFound           = New(400001, "order not found")
	ErrInvalidStatusTransition = New(400002, "invalid status transition")
)
