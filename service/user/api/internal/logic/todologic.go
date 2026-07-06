package logic

import (
	"context"
	"errors"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/common/model"
	"github.com/fx0x55/micro-go-lab/common/page"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// 保留向后兼容的别名
var ErrTodoNotFound = ecode.ErrTodoNotFound

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
	if err := l.svcCtx.TodoRepo.Create(l.ctx, todo); err != nil {
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

func (l *ListTodoLogic) ListByUserID(userID uint, pg, pageSize int) (*page.Result, error) {
	todos, total, err := l.svcCtx.TodoRepo.FindByUserIDWithPage(l.ctx, userID, (pg-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	return page.NewResult(todos, total, pg, pageSize), nil
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
	todo, err := l.svcCtx.TodoRepo.FindByIDAndUserID(l.ctx, id, userID)
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
	// 先查询用于返回结果
	todo, err := l.svcCtx.TodoRepo.FindByIDAndUserID(l.ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, err
	}

	title := todo.Title
	completed := todo.Completed
	if req.Title != nil {
		title = *req.Title
	}
	if req.Completed != nil {
		completed = *req.Completed
	}

	if err := l.svcCtx.TodoRepo.Update(l.ctx, userID, id, title, completed); err != nil {
		return nil, err
	}

	todo.Title = title
	todo.Completed = completed
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
	return l.svcCtx.TodoRepo.Delete(l.ctx, userID, id)
}
