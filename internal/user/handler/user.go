package handler

import (
	"errors"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/user/service"
)

type UserHandler struct {
	userSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

func (h *UserHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterRequest
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, err.Error())
		return
	}

	user, err := h.userSvc.Register(&req)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			middleware.ErrorJson(w, http.StatusConflict, err.Error())
			return
		}
		middleware.InternalError(w, "registration failed")
		return
	}

	middleware.CreatedJson(w, user)
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, err.Error())
		return
	}

	resp, err := h.userSvc.Login(&req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			middleware.Unauthorized(w, err.Error())
			return
		}
		middleware.InternalError(w, "login failed")
		return
	}

	middleware.OkJson(w, resp)
}

func (h *UserHandler) Profile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	user, err := h.userSvc.GetByID(userID)
	if err != nil {
		middleware.NotFound(w, "user not found")
		return
	}

	middleware.OkJson(w, user)
}
