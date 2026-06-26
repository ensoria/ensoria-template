package service

import (
	"github.com/ensoria/ensoria-template/internal/query/user_post/record"
	"github.com/ensoria/ensoria-template/internal/query/user_post/repository"
)

//ensoria:mock
type UserPostService interface {
	GetByID(id int64) *record.UserPostRecord
}

func NewUserPostService(
	repository repository.UserPostRepository,
) *userPostService {
	return &userPostService{
		repository: repository,
	}
}

type userPostService struct {
	repository repository.UserPostRepository
}

func (s *userPostService) GetByID(id int64) *record.UserPostRecord {
	return s.repository.GetByID(id)
}
