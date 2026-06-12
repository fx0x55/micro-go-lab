package handler

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/user/service"
)

type TodoHandler struct {
	todoSvc *service.TodoService
}

func NewTodoHandler(todoSvc *service.TodoService) *TodoHandler {
	return &TodoHandler{todoSvc: todoSvc}
}

func (h *TodoHandler) Create(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	var req service.CreateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequest(c, err.Error())
		return
	}

	todo, err := h.todoSvc.Create(userID, &req)
	if err != nil {
		middleware.InternalError(c, "failed to create todo")
		return
	}

	middleware.Created(c, todo)
}

func (h *TodoHandler) List(c *gin.Context) {
	userID := c.MustGet("user_id").(uint)

	todos, err := h.todoSvc.ListByUserID(userID)
	if err != nil {
		middleware.InternalError(c, "failed to list todos")
		return
	}

	middleware.Success(c, todos)
}

func (h *TodoHandler) Get(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		middleware.BadRequest(c, "invalid id")
		return
	}

	todo, err := h.todoSvc.GetByID(id)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			middleware.NotFound(c, err.Error())
			return
		}
		middleware.InternalError(c, "failed to get todo")
		return
	}

	middleware.Success(c, todo)
}

func (h *TodoHandler) Update(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		middleware.BadRequest(c, "invalid id")
		return
	}

	var req service.UpdateTodoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		middleware.BadRequest(c, err.Error())
		return
	}

	todo, err := h.todoSvc.Update(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			middleware.NotFound(c, err.Error())
			return
		}
		middleware.InternalError(c, "failed to update todo")
		return
	}

	middleware.Success(c, todo)
}

func (h *TodoHandler) Delete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		middleware.BadRequest(c, "invalid id")
		return
	}

	if err := h.todoSvc.Delete(id); err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			middleware.NotFound(c, err.Error())
			return
		}
		middleware.InternalError(c, "failed to delete todo")
		return
	}

	middleware.Success(c, nil)
}

func parseID(c *gin.Context) (uint, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	return uint(id), err
}
