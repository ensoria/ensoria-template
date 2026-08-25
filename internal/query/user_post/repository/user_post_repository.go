package repository

import (
	"context"
	"time"

	"github.com/ensoria/ensoria-template/internal/query/user_post/record"
)

//ensoria:mock
type UserPostRepository interface {
	// GetByID reads one record.
	//
	// It takes a context and returns an error because a real repository talks
	// to a database: the caller has to be able to cancel the read and to hear
	// about a failed one. The stub below ignores both, but the signature is
	// what the service is written against.
	GetByID(ctx context.Context, id int64) (*record.UserPostRecord, error)
}

type userPostRepository struct{}

func NewUserPostRepository() *userPostRepository {
	return &userPostRepository{}
}

func (r *userPostRepository) GetByID(ctx context.Context, id int64) (*record.UserPostRecord, error) {
	return &record.UserPostRecord{
		ID:        id,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}
