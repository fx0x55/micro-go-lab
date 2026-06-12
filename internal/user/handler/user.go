package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/user/service"
)

type UserHandler struct {
	userSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req service.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequest(c, err.Error())
		return
	}

	user, err := h.userSvc.Register(&req)
	if err != nil {
		if errors.Is(err, service.ErrUserExists) {
			middleware.Error(c, 409, err.Error())
			return
		}
		middleware.InternalError(c, "registration failed")
		return
	}

	middleware.Created(c, user)
}

func (h *UserHandler) Login(c *gin.Context) {
	var req service.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequest(c, err.Error())
		return
	}

	resp, err := h.userSvc.Login(&req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			middleware.Unauthorized(c, err.Error())
			return
		}
		middleware.InternalError(c, "login failed")
		return
	}

	middleware.Success(c, resp)
}

func (h *UserHandler) Profile(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	user, err := h.userSvc.GetByID(userID)
	if err != nil {
		middleware.NotFound(c, "user not found")
		return
	}

	middleware.Success(c, user)
}
