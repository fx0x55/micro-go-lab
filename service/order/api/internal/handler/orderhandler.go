package handler

import (
	"errors"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/wokoworks/go-server/common/middleware"
	"github.com/wokoworks/go-server/service/order/api/internal/logic"
	"github.com/wokoworks/go-server/service/order/api/internal/svc"
	"github.com/wokoworks/go-server/service/order/api/internal/types"
)

func CreateOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.CreateOrderRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logic.NewCreateOrderLogic(r.Context(), svcCtx)
		order, err := l.Create(userID, &req)
		if err != nil {
			if errors.Is(err, logic.ErrUserNotFound) {
				middleware.BadRequest(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to create order")
			return
		}

		middleware.CreatedJson(w, order)
	}
}

func GetOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.OrderIDReq
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, "invalid id")
			return
		}

		l := logic.NewGetOrderLogic(r.Context(), svcCtx)
		order, err := l.GetByID(userID, req.ID)
		if err != nil {
			if errors.Is(err, logic.ErrOrderNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to get order")
			return
		}

		middleware.OkJson(w, order)
	}
}

func ListOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.ListOrderRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logic.NewListOrderLogic(r.Context(), svcCtx)
		result, err := l.ListByUserID(userID, req.Page, req.PageSize)
		if err != nil {
			middleware.InternalError(w, "failed to list orders")
			return
		}

		middleware.OkJson(w, result)
	}
}

func UpdateOrderStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var pathReq types.OrderIDReq
		if err := httpx.Parse(r, &pathReq); err != nil {
			middleware.BadRequest(w, "invalid id")
			return
		}

		var req types.UpdateOrderStatusRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logic.NewUpdateOrderStatusLogic(r.Context(), svcCtx)
		order, err := l.UpdateStatus(userID, pathReq.ID, &req)
		if err != nil {
			if errors.Is(err, logic.ErrOrderNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			if errors.Is(err, logic.ErrInvalidStatusTransition) {
				middleware.BadRequest(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to update order")
			return
		}

		middleware.OkJson(w, order)
	}
}
