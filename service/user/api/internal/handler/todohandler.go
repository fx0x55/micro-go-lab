package handler

import (
	"errors"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/wokoworks/go-server/common/middleware"
	"github.com/wokoworks/go-server/service/user/api/internal/logic"
	"github.com/wokoworks/go-server/service/user/api/internal/svc"
	"github.com/wokoworks/go-server/service/user/api/internal/types"
)

func CreateTodoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.CreateTodoRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logic.NewCreateTodoLogic(r.Context(), svcCtx)
		todo, err := l.Create(userID, &req)
		if err != nil {
			middleware.InternalError(w, "failed to create todo")
			return
		}

		middleware.CreatedJson(w, todo)
	}
}

func ListTodoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		l := logic.NewListTodoLogic(r.Context(), svcCtx)
		todos, err := l.ListByUserID(userID)
		if err != nil {
			middleware.InternalError(w, "failed to list todos")
			return
		}

		middleware.OkJson(w, todos)
	}
}

func GetTodoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.TodoIDReq
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, "invalid id")
			return
		}

		l := logic.NewGetTodoLogic(r.Context(), svcCtx)
		todo, err := l.GetByID(userID, req.ID)
		if err != nil {
			if errors.Is(err, logic.ErrTodoNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to get todo")
			return
		}

		middleware.OkJson(w, todo)
	}
}

func UpdateTodoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var pathReq types.TodoIDReq
		if err := httpx.Parse(r, &pathReq); err != nil {
			middleware.BadRequest(w, "invalid id")
			return
		}

		var req types.UpdateTodoRequest
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, err.Error())
			return
		}

		l := logic.NewUpdateTodoLogic(r.Context(), svcCtx)
		todo, err := l.Update(userID, pathReq.ID, &req)
		if err != nil {
			if errors.Is(err, logic.ErrTodoNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to update todo")
			return
		}

		middleware.OkJson(w, todo)
	}
}

func DeleteTodoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserID(r)

		var req types.TodoIDReq
		if err := httpx.Parse(r, &req); err != nil {
			middleware.BadRequest(w, "invalid id")
			return
		}

		l := logic.NewDeleteTodoLogic(r.Context(), svcCtx)
		if err := l.Delete(userID, req.ID); err != nil {
			if errors.Is(err, logic.ErrTodoNotFound) {
				middleware.NotFound(w, err.Error())
				return
			}
			middleware.InternalError(w, "failed to delete todo")
			return
		}

		middleware.OkJson(w, nil)
	}
}
