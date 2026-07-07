package user

import (
	"errors"
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	logiclib "github.com/fx0x55/micro-go-lab/service/user/api/internal/logic/user"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func RegisterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RegisterRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logiclib.NewRegisterLogic(r.Context(), svcCtx)
		resp, err := l.Register(&req)
		if err != nil {
			if errors.Is(err, logiclib.ErrUserExists) {
				middleware.ErrorJson(w, http.StatusConflict, err.Error())
				return
			}
			middleware.InternalError(w, "registration failed")
			return
		}
		middleware.CreatedJson(w, resp)
	}
}
