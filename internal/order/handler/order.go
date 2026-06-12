package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/order/service"
)

type OrderHandler struct {
	orderSvc *service.OrderService
}

func NewOrderHandler(orderSvc *service.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

func (h *OrderHandler) Create(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req service.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequest(c, err.Error())
		return
	}

	order, err := h.orderSvc.Create(c.Request.Context(), userID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			middleware.BadRequest(c, err.Error())
			return
		}
		middleware.InternalError(c, "failed to create order")
		return
	}

	middleware.Created(c, order)
}

func (h *OrderHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		middleware.BadRequest(c, "invalid id")
		return
	}

	order, err := h.orderSvc.GetByID(id)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			middleware.NotFound(c, err.Error())
			return
		}
		middleware.InternalError(c, "failed to get order")
		return
	}

	middleware.Success(c, order)
}

func (h *OrderHandler) List(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	orders, err := h.orderSvc.ListByUserID(userID)
	if err != nil {
		middleware.InternalError(c, "failed to list orders")
		return
	}

	middleware.Success(c, orders)
}

func (h *OrderHandler) UpdateStatus(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		middleware.BadRequest(c, "invalid id")
		return
	}

	var req service.UpdateOrderStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequest(c, err.Error())
		return
	}

	order, err := h.orderSvc.UpdateStatus(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrOrderNotFound) {
			middleware.NotFound(c, err.Error())
			return
		}
		middleware.InternalError(c, "failed to update order")
		return
	}

	middleware.Success(c, order)
}

func parseID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id), err
}
