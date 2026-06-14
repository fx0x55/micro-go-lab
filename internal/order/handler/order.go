package handler

import (
	"errors"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/order/service"
)

type OrderHandler struct {
	orderSvc *service.OrderService
}

func NewOrderHandler(orderSvc *service.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

type orderIDReq struct {
	ID uint `path:"id"`
}

func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var req service.CreateOrderRequest
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, err.Error())
		return
	}

	order, err := h.orderSvc.Create(r.Context(), userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			middleware.BadRequest(w, err.Error())
			return
		}
		middleware.InternalError(w, "failed to create order")
		return
	}

	middleware.CreatedJson(w, order)
}

func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var req orderIDReq
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, "invalid id")
		return
	}

	order, err := h.orderSvc.GetByID(userID, req.ID)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			middleware.NotFound(w, err.Error())
			return
		}
		middleware.InternalError(w, "failed to get order")
		return
	}

	middleware.OkJson(w, order)
}

func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	orders, err := h.orderSvc.ListByUserID(userID)
	if err != nil {
		middleware.InternalError(w, "failed to list orders")
		return
	}

	middleware.OkJson(w, orders)
}

func (h *OrderHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	var pathReq orderIDReq
	if err := httpx.Parse(r, &pathReq); err != nil {
		middleware.BadRequest(w, "invalid id")
		return
	}

	var req service.UpdateOrderStatusRequest
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, err.Error())
		return
	}

	order, err := h.orderSvc.UpdateStatus(userID, pathReq.ID, &req)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			middleware.NotFound(w, err.Error())
			return
		}
		if errors.Is(err, service.ErrInvalidStatusTransition) {
			middleware.BadRequest(w, err.Error())
			return
		}
		middleware.InternalError(w, "failed to update order")
		return
	}

	middleware.OkJson(w, order)
}
