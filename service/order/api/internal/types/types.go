package types

type CreateOrderRequest struct {
	ProductName string `json:"product_name" validate:"required,min=1,max=256"`
	Amount      int64  `json:"amount" validate:"required,gt=0"`
}

type UpdateOrderStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=paid cancelled"`
}

type OrderIDReq struct {
	ID uint `path:"id"`
}

type ListOrderRequest struct {
	Page     int `form:"page,default=1" validate:"min=1"`
	PageSize int `form:"page_size,default=20" validate:"min=1,max=100"`
}
