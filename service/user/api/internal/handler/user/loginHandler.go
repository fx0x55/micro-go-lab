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

func LoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.LoginRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logiclib.NewLoginLogic(r.Context(), svcCtx)
		resp, err := l.Login(&req)
		if err != nil {
			if errors.Is(err, logiclib.ErrInvalidCredentials) {
				middleware.Unauthorized(w, err.Error())
				return
			}
			middleware.InternalError(w, "login failed")
			return
		}
		middleware.OkJson(w, resp)
	}
}
