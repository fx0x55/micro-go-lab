package logic

import (
	"context"
	"errors"

	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"

	"github.com/wokoworks/go-server/common/model"
	"github.com/wokoworks/go-server/service/user/api/internal/svc"
	"github.com/wokoworks/go-server/service/user/api/internal/types"
)

var ErrTodoNotFound = errors.New("todo not found")

type CreateTodoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateTodoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateTodoLogic {
	return &CreateTodoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateTodoLogic) Create(userID uint, req *types.CreateTodoRequest) (*model.Todo, error) {
	todo := &model.Todo{
		UserID: userID,
		Title:  req.Title,
	}
	if err := l.svcCtx.TodoRepo.Create(todo); err != nil {
		return nil, err
	}
	return todo, nil
}

type ListTodoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListTodoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListTodoLogic {
	return &ListTodoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListTodoLogic) ListByUserID(userID uint) ([]model.Todo, error) {
	return l.svcCtx.TodoRepo.FindByUserID(userID)
}

type GetTodoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTodoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTodoLogic {
	return &GetTodoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetTodoLogic) GetByID(userID, id uint) (*model.Todo, error) {
	todo, err := l.svcCtx.TodoRepo.FindByIDAndUserID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, err
	}
	return todo, nil
}

type UpdateTodoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateTodoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateTodoLogic {
	return &UpdateTodoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateTodoLogic) Update(userID, id uint, req *types.UpdateTodoRequest) (*model.Todo, error) {
	todo, err := l.svcCtx.TodoRepo.FindByIDAndUserID(id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, err
	}

	if req.Title != nil {
		todo.Title = *req.Title
	}
	if req.Completed != nil {
		todo.Completed = *req.Completed
	}

	if err := l.svcCtx.TodoRepo.Update(todo); err != nil {
		return nil, err
	}
	return todo, nil
}

type DeleteTodoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteTodoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteTodoLogic {
	return &DeleteTodoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteTodoLogic) Delete(userID, id uint) error {
	if _, err := l.svcCtx.TodoRepo.FindByIDAndUserID(id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTodoNotFound
		}
		return err
	}
	return l.svcCtx.TodoRepo.Delete(id)
}
