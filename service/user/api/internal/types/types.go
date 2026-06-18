package types

type RegisterRequest struct {
	Username string `json:"username" validate:"required,min=3,max=64"`
	Password string `json:"password" validate:"required,min=6,max=128"`
	Email    string `json:"email" validate:"required,email"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type CreateTodoRequest struct {
	Title string `json:"title" validate:"required,min=1,max=256"`
}

type UpdateTodoRequest struct {
	Title     *string `json:"title" validate:"omitempty,min=1,max=256"`
	Completed *bool   `json:"completed"`
}

type TodoIDReq struct {
	ID uint `path:"id"`
}

type ListTodoRequest struct {
	Page     int `form:"page,default=1" validate:"min=1"`
	PageSize int `form:"page_size,default=20" validate:"min=1,max=100"`
}
