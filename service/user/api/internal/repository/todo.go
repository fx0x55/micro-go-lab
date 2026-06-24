package repository

import (
	"context"

	"github.com/fx0x55/micro-go-lab/common/ecode"
	"github.com/fx0x55/micro-go-lab/common/model"
	"gorm.io/gorm"
)

// TodoRepositoryInterface 定义待办事项数据访问层的接口
// 使用接口而不是具体类型，便于测试时mock
type TodoRepositoryInterface interface {
	Create(ctx context.Context, todo *model.Todo) error
	FindByIDAndUserID(ctx context.Context, id, userID uint) (*model.Todo, error)
	FindByUserID(ctx context.Context, userID uint) ([]model.Todo, error)
	FindByUserIDWithPage(ctx context.Context, userID uint, offset, limit int) ([]model.Todo, int64, error)
	Update(ctx context.Context, todo *model.Todo) error
	Delete(ctx context.Context, id uint) error
}

type TodoRepository struct {
	db *gorm.DB
}

func NewTodoRepository(db *gorm.DB) *TodoRepository {
	return &TodoRepository{db: db}
}

func (r *TodoRepository) Create(ctx context.Context, todo *model.Todo) error {
	return r.db.WithContext(ctx).Create(todo).Error
}

func (r *TodoRepository) FindByIDAndUserID(ctx context.Context, id, userID uint) (*model.Todo, error) {
	var todo model.Todo
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&todo).Error
	if err != nil {
		return nil, err
	}
	return &todo, nil
}

func (r *TodoRepository) FindByUserID(ctx context.Context, userID uint) ([]model.Todo, error) {
	var todos []model.Todo
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&todos).Error
	return todos, err
}

func (r *TodoRepository) FindByUserIDWithPage(
	ctx context.Context,
	userID uint,
	offset, limit int,
) ([]model.Todo, int64, error) {
	var todos []model.Todo
	var total int64

	db := r.db.WithContext(ctx).Model(&model.Todo{}).Where("user_id = ?", userID)
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := db.Order("created_at DESC").Offset(offset).Limit(limit).Find(&todos).Error
	return todos, total, err
}

func (r *TodoRepository) Update(ctx context.Context, todo *model.Todo) error {
	result := r.db.WithContext(ctx).Save(todo)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ecode.ErrOptimisticConflict
	}
	return nil
}

func (r *TodoRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.Todo{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ecode.ErrTodoNotFound
	}
	return nil
}
