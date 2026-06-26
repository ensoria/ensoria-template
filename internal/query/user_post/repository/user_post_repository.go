package repository

import (
	"time"

	"github.com/ensoria/ensoria-template/internal/query/user_post/record"
)

//ensoria:mock
type UserPostRepository interface {
	GetByID(id int64) *record.UserPostRecord
}

type userPostRepository struct{}

func NewUserPostRepository() *userPostRepository {
	return &userPostRepository{}
}

func (r *userPostRepository) GetByID(id int64) *record.UserPostRecord {
	return &record.UserPostRecord{
		ID:        1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}
