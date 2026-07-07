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

func GetOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.OrderIDReq
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, "invalid id")
			return
		}

		l := logiclib.NewGetOrderLogic(r.Context(), svcCtx)
		order, err := l.GetByID(userID, req.ID)
		if err != nil {
			if errors.Is(err, logiclib.ErrOrderNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to get order")
			return
		}
		middleware.OkJson(w, order)
	}
}
