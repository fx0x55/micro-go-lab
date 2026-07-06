package handler

import (
	"errors"
	"net/http"

	"github.com/fx0x55/micro-go-lab/common/middleware"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/logic"
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

		l := logic.NewRegisterLogic(r.Context(), svcCtx)
		user, err := l.Register(&req)
		if err != nil {
			if errors.Is(err, logic.ErrUserExists) {
				middleware.ErrorJson(w, http.StatusConflict, err.Error())
				return
			}
			middleware.InternalError(w, "registration failed")
			return
		}

		middleware.CreatedJson(w, user)
	}
}

func LoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.LoginRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logic.NewLoginLogic(r.Context(), svcCtx)
		resp, err := l.Login(&req)
		if err != nil {
			if errors.Is(err, logic.ErrInvalidCredentials) {
				middleware.Unauthorized(w, err.Error())
				return
			}
			middleware.InternalError(w, "login failed")
			return
		}
		middleware.OkJson(w, resp)
	}
}

func ProfileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		l := logic.NewProfileLogic(r.Context(), svcCtx)
		user, err := l.GetProfile(userID)
		if err != nil {
			middleware.NotFound(w, "user not found")
			return
		}

		middleware.OkJson(w, user)
	}
}
