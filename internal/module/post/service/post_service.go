package service

import "github.com/ensoria/ensoria-template/internal/module/post/dto"

//ensoria:mock
type PostService interface {
	GetPost() *dto.Post
}

func NewPostService() *postService {
	return &postService{}
}

type postService struct {
}

func (s *postService) GetPost() *dto.Post {
	return &dto.Post{
		ID:      1,
		UserID:  1,
		Title:   "Hello World",
		Content: "This is a sample post.",
	}
}
