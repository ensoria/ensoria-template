package repository

import (
	"time"

	"github.com/ensoria/ensoria-template/internal/module/user/model"
)

type UserRepository interface {
	GetByID(id int64) (*model.User, error)
}

type userRepository struct{}

func NewUserRepository() *userRepository {
	return &userRepository{}
}

func (r *userRepository) GetByID(id int64) (*model.User, error) {
	return &model.User{
		ID:        1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}
