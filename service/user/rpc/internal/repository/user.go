package repository

import (
	"context"

	"github.com/fx0x55/micro-go-lab/service/user/rpc/internal/model"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Create 写入用户（需在业务事务中调用，与 outbox 写入同事务）。
func (r *UserRepository) Create(tx *gorm.DB, user *model.User) error {
	return tx.Create(user).Error
}

// FindByUsername 按用户名查找；不存在返回 gorm.ErrRecordNotFound。
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByEmail 按邮箱查找；不存在返回 gorm.ErrRecordNotFound。
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
