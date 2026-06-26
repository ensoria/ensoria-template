package service

//ensoria:mock
type PostService interface {
	Anything() string
}

func NewPostService() *postService {
	return &postService{}
}

type postService struct {
}

func (s *postService) Anything() string {
	return "post service response"
}
