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
