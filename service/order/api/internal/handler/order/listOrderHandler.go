package order

import (
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	logiclib "github.com/fx0x55/micro-go-lab/service/order/api/internal/logic/order"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/order/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func ListOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.ListOrderRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logiclib.NewListOrderLogic(r.Context(), svcCtx)
		result, err := l.ListByUserID(userID, req.Page, req.PageSize)
		if err != nil {
			middleware.InternalError(w, "failed to list orders")
			return
		}
		middleware.OkJson(w, result)
	}
}
