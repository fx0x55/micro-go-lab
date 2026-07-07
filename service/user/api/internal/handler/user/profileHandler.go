package user

import (
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/logic/user"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
)

func ProfileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := user.NewProfileLogic(r.Context(), svcCtx)
		resp, err := l.Profile()
		if err != nil {
			middleware.NotFound(w, "user not found")
			return
		}
		middleware.OkJson(w, resp)
	}
}
