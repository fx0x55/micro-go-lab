package logic

import (
	"context"
	"testing"
	"time"

	"github.com/fx0x55/micro-go-lab/common/model"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/config"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/mocks"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/svc"
	"github.com/fx0x55/micro-go-lab/service/user/api/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

const testTodoTitle = "Test Todo"

func TestCreateTodoLogic_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTodoRepo := mocks.NewMockTodoRepositoryInterface(ctrl)
	svcCtx := &svc.ServiceContext{
		Config:   config.Config{},
		TodoRepo: mockTodoRepo,
	}

	testTodo := &model.Todo{
		BaseModel: model.BaseModel{Model: gorm.Model{ID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}},
		UserID:    1,
		Title:     testTodoTitle,
		Completed: false,
	}

	tests := []struct {
		name         string
		userID       uint
		req          *types.CreateTodoRequest
		mockSetup    func()
		expectedTodo *model.Todo
		expectedErr  error
	}{
		{
			name:   "创建成功",
			userID: 1,
			req:    &types.CreateTodoRequest{Title: testTodoTitle},
			mockSetup: func() {
				mockTodoRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, todo *model.Todo) error {
						todo.ID = testTodo.ID
						todo.CreatedAt = testTodo.CreatedAt
						todo.UpdatedAt = testTodo.UpdatedAt
						return nil
					}).
					Times(1)
			},
			expectedTodo: testTodo,
			expectedErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			logic := NewCreateTodoLogic(context.Background(), svcCtx)
			todo, err := logic.Create(tt.userID, tt.req)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, todo)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, todo)
			assert.Equal(t, tt.expectedTodo.Title, todo.Title)
			assert.Equal(t, tt.userID, todo.UserID)
		})
	}
}

func TestGetTodoLogic_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTodoRepo := mocks.NewMockTodoRepositoryInterface(ctrl)
	svcCtx := &svc.ServiceContext{
		Config:   config.Config{},
		TodoRepo: mockTodoRepo,
	}

	testTodo := &model.Todo{
		BaseModel: model.BaseModel{Model: gorm.Model{ID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}},
		UserID:    1,
		Title:     testTodoTitle,
		Completed: false,
	}

	tests := []struct {
		name         string
		userID       uint
		todoID       uint
		mockSetup    func()
		expectedTodo *model.Todo
		expectedErr  error
	}{
		{
			name:   "查询成功",
			userID: 1,
			todoID: 1,
			mockSetup: func() {
				mockTodoRepo.EXPECT().
					FindByIDAndUserID(gomock.Any(), uint(1), uint(1)).
					Return(testTodo, nil).
					Times(1)
			},
			expectedTodo: testTodo,
			expectedErr:  nil,
		},
		{
			name:   "记录不存在",
			userID: 1,
			todoID: 999,
			mockSetup: func() {
				mockTodoRepo.EXPECT().
					FindByIDAndUserID(gomock.Any(), uint(999), uint(1)).
					Return(nil, gorm.ErrRecordNotFound).
					Times(1)
			},
			expectedTodo: nil,
			expectedErr:  ErrTodoNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			logic := NewGetTodoLogic(context.Background(), svcCtx)
			todo, err := logic.GetByID(tt.userID, tt.todoID)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, todo)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, todo)
			assert.Equal(t, tt.expectedTodo.ID, todo.ID)
			assert.Equal(t, tt.expectedTodo.Title, todo.Title)
		})
	}
}

func TestListTodoLogic_ListByUserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTodoRepo := mocks.NewMockTodoRepositoryInterface(ctrl)
	svcCtx := &svc.ServiceContext{
		Config:   config.Config{},
		TodoRepo: mockTodoRepo,
	}

	testTodos := []model.Todo{
		{BaseModel: model.BaseModel{Model: gorm.Model{ID: 1, CreatedAt: time.Now()}}, UserID: 1, Title: "Todo 1"},
		{BaseModel: model.BaseModel{Model: gorm.Model{ID: 2, CreatedAt: time.Now()}}, UserID: 1, Title: "Todo 2"},
	}

	tests := []struct {
		name        string
		userID      uint
		page        int
		pageSize    int
		mockSetup   func()
		expectedErr error
	}{
		{
			name:     "分页查询成功",
			userID:   1,
			page:     1,
			pageSize: 10,
			mockSetup: func() {
				mockTodoRepo.EXPECT().
					FindByUserIDWithPage(gomock.Any(), uint(1), 0, 10).
					Return(testTodos, int64(2), nil).
					Times(1)
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			logic := NewListTodoLogic(context.Background(), svcCtx)
			result, err := logic.ListByUserID(tt.userID, tt.page, tt.pageSize)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, int64(2), result.Total)
			assert.Equal(t, tt.page, result.Page)
			assert.Equal(t, tt.pageSize, result.PageSize)
		})
	}
}

func TestUpdateTodoLogic_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTodoRepo := mocks.NewMockTodoRepositoryInterface(ctrl)
	svcCtx := &svc.ServiceContext{
		Config:   config.Config{},
		TodoRepo: mockTodoRepo,
	}

	testTodo := &model.Todo{
		BaseModel: model.BaseModel{Model: gorm.Model{ID: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}},
		UserID:    1,
		Title:     "Old Title",
		Completed: false,
	}

	newTitle := "New Title"
	newCompleted := true

	tests := []struct {
		name         string
		userID       uint
		todoID       uint
		req          *types.UpdateTodoRequest
		mockSetup    func()
		expectedTodo *model.Todo
		expectedErr  error
	}{
		{
			name:   "更新成功",
			userID: 1,
			todoID: 1,
			req: &types.UpdateTodoRequest{
				Title:     &newTitle,
				Completed: &newCompleted,
			},
			mockSetup: func() {
				// Mock FindByIDAndUserID
				mockTodoRepo.EXPECT().
					FindByIDAndUserID(gomock.Any(), uint(1), uint(1)).
					Return(testTodo, nil).
					Times(1)

				// Mock Update
				mockTodoRepo.EXPECT().
					Update(gomock.Any(), uint(1), uint(1), newTitle, newCompleted).
					Return(nil).
					Times(1)
			},
			expectedTodo: &model.Todo{
				BaseModel: model.BaseModel{
					Model: gorm.Model{ID: 1, CreatedAt: testTodo.CreatedAt, UpdatedAt: testTodo.UpdatedAt},
				},
				UserID:    1,
				Title:     newTitle,
				Completed: newCompleted,
			},
			expectedErr: nil,
		},
		{
			name:   "记录不存在",
			userID: 1,
			todoID: 999,
			req: &types.UpdateTodoRequest{
				Title: &newTitle,
			},
			mockSetup: func() {
				mockTodoRepo.EXPECT().
					FindByIDAndUserID(gomock.Any(), uint(999), uint(1)).
					Return(nil, gorm.ErrRecordNotFound).
					Times(1)
			},
			expectedTodo: nil,
			expectedErr:  ErrTodoNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			logic := NewUpdateTodoLogic(context.Background(), svcCtx)
			todo, err := logic.Update(tt.userID, tt.todoID, tt.req)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				assert.Nil(t, todo)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, todo)
			assert.Equal(t, tt.expectedTodo.Title, todo.Title)
			assert.Equal(t, tt.expectedTodo.Completed, todo.Completed)
		})
	}
}

func TestDeleteTodoLogic_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTodoRepo := mocks.NewMockTodoRepositoryInterface(ctrl)
	svcCtx := &svc.ServiceContext{
		Config:   config.Config{},
		TodoRepo: mockTodoRepo,
	}

	tests := []struct {
		name        string
		userID      uint
		todoID      uint
		mockSetup   func()
		expectedErr error
	}{
		{
			name:   "删除成功",
			userID: 1,
			todoID: 1,
			mockSetup: func() {
				// Mock Delete (single SQL with ownership check)
				mockTodoRepo.EXPECT().
					Delete(gomock.Any(), uint(1), uint(1)).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name:   "记录不存在",
			userID: 1,
			todoID: 999,
			mockSetup: func() {
				mockTodoRepo.EXPECT().
					Delete(gomock.Any(), uint(1), uint(999)).
					Return(ErrTodoNotFound).
					Times(1)
			},
			expectedErr: ErrTodoNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			logic := NewDeleteTodoLogic(context.Background(), svcCtx)
			err := logic.Delete(tt.userID, tt.todoID)

			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
				return
			}

			require.NoError(t, err)
		})
	}
}
