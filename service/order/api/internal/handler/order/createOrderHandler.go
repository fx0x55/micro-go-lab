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

func CreateOrderHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.CreateOrderRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logiclib.NewCreateOrderLogic(r.Context(), svcCtx)
		idemKey := r.Header.Get("Idempotency-Key")
		order, err := l.Create(userID, &req, idemKey)
		if err != nil {
			if errors.Is(err, logiclib.ErrIdempotencyConflict) {
				middleware.ErrorJson(w, http.StatusConflict, err.Error())
				return
			}
			if errors.Is(err, logiclib.ErrUserNotFound) {
				middleware.BadRequest(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to create order")
			return
		}
		middleware.CreatedJson(w, order)
	}
}
