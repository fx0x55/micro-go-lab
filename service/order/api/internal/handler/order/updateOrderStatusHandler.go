package order

import (
	"errors"
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	logiclib "github.com/fx0x55/micro-go-lab/service/order/api/internal/logic/order"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

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

		l := logiclib.NewUpdateOrderStatusLogic(r.Context(), svcCtx)
		order, err := l.UpdateStatus(userID, pathReq.ID, &req)
		if err != nil {
			if errors.Is(err, logiclib.ErrOrderNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			if errors.Is(err, logiclib.ErrInvalidStatusTransition) {
				middleware.BadRequest(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to update order")
			return
		}
		middleware.OkJson(w, order)
	}
}
