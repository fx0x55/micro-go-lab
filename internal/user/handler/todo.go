package handler

import (
	"errors"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/wokoworks/go-server/internal/middleware"
	"github.com/wokoworks/go-server/internal/user/service"
)

type TodoHandler struct {
	todoSvc *service.TodoService
}

func NewTodoHandler(todoSvc *service.TodoService) *TodoHandler {
	return &TodoHandler{todoSvc: todoSvc}
}

type todoIDReq struct {
	ID uint `path:"id"`
}

func (h *TodoHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req service.CreateTodoRequest
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, err.Error())
		return
	}

	todo, err := h.todoSvc.Create(userID, &req)
	if err != nil {
		middleware.InternalError(w, "failed to create todo")
		return
	}

	middleware.CreatedJson(w, todo)
}

func (h *TodoHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	todos, err := h.todoSvc.ListByUserID(userID)
	if err != nil {
		middleware.InternalError(w, "failed to list todos")
		return
	}

	middleware.OkJson(w, todos)
}

func (h *TodoHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req todoIDReq
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, "invalid id")
		return
	}

	todo, err := h.todoSvc.GetByID(userID, req.ID)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			middleware.NotFound(w, err.Error())
			return
		}
		middleware.InternalError(w, "failed to get todo")
		return
	}

	middleware.OkJson(w, todo)
}

func (h *TodoHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var pathReq todoIDReq
	if err := httpx.Parse(r, &pathReq); err != nil {
		middleware.BadRequest(w, "invalid id")
		return
	}

	var req service.UpdateTodoRequest
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, err.Error())
		return
	}

	todo, err := h.todoSvc.Update(userID, pathReq.ID, &req)
	if err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			middleware.NotFound(w, err.Error())
			return
		}
		middleware.InternalError(w, "failed to update todo")
		return
	}

	middleware.OkJson(w, todo)
}

func (h *TodoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req todoIDReq
	if err := httpx.Parse(r, &req); err != nil {
		middleware.BadRequest(w, "invalid id")
		return
	}

	if err := h.todoSvc.Delete(userID, req.ID); err != nil {
		if errors.Is(err, service.ErrTodoNotFound) {
			middleware.NotFound(w, err.Error())
			return
		}
		middleware.InternalError(w, "failed to delete todo")
		return
	}

	middleware.OkJson(w, nil)
}
