package repository

import (
	"github.com/wokoworks/go-server/common/model"
	"gorm.io/gorm"
)

type TodoRepository struct {
	db *gorm.DB
}

func NewTodoRepository(db *gorm.DB) *TodoRepository {
	return &TodoRepository{db: db}
}

func (r *TodoRepository) Create(todo *model.Todo) error {
	return r.db.Create(todo).Error
}

func (r *TodoRepository) FindByIDAndUserID(id, userID uint) (*model.Todo, error) {
	var todo model.Todo
	err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&todo).Error
	if err != nil {
		return nil, err
	}
	return &todo, nil
}

func (r *TodoRepository) FindByUserID(userID uint) ([]model.Todo, error) {
	var todos []model.Todo
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&todos).Error
	return todos, err
}

func (r *TodoRepository) Update(todo *model.Todo) error {
	return r.db.Save(todo).Error
}

func (r *TodoRepository) Delete(id uint) error {
	return r.db.Delete(&model.Todo{}, id).Error
}
