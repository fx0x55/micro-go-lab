package service

import (
	"errors"

	"github.com/wokoworks/go-server/internal/user/model"
	"github.com/wokoworks/go-server/internal/user/repository"
	"gorm.io/gorm"
)

var ErrTodoNotFound = errors.New("todo not found")

type TodoService struct {
	todoRepo *repository.TodoRepository
}

func NewTodoService(todoRepo *repository.TodoRepository) *TodoService {
	return &TodoService{todoRepo: todoRepo}
}

type CreateTodoRequest struct {
	Title string `json:"title" validate:"required,min=1,max=256"`
}

type UpdateTodoRequest struct {
	Title     *string `json:"title" validate:"omitempty,min=1,max=256"`
	Completed *bool   `json:"completed"`
}

func (s *TodoService) Create(userID uint, req *CreateTodoRequest) (*model.Todo, error) {
	todo := &model.Todo{
		UserID: userID,
		Title:  req.Title,
	}
	if err := s.todoRepo.Create(todo); err != nil {
		return nil, err
	}
	return todo, nil
}

func (s *TodoService) GetByID(id uint) (*model.Todo, error) {
	todo, err := s.todoRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTodoNotFound
		}
		return nil, err
	}
	return todo, nil
}

func (s *TodoService) ListByUserID(userID uint) ([]model.Todo, error) {
	return s.todoRepo.FindByUserID(userID)
}

func (s *TodoService) Update(id uint, req *UpdateTodoRequest) (*model.Todo, error) {
	todo, err := s.todoRepo.FindByID(id)
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

	if err := s.todoRepo.Update(todo); err != nil {
		return nil, err
	}
	return todo, nil
}

func (s *TodoService) Delete(id uint) error {
	_, err := s.todoRepo.FindByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTodoNotFound
		}
		return err
	}
	return s.todoRepo.Delete(id)
}
