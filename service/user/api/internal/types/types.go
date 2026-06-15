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
