package service

import (
	"context"
	"fmt"
	"time"

	order "github.com/ensoria/ensoria-template/internal/module/order/service"
	"github.com/ensoria/ensoria-template/internal/module/user/model"
	"github.com/ensoria/ensoria-template/internal/module/user/repository"
	pbPost "github.com/ensoria/ensoria-template/pb/post"
	"github.com/ensoria/worker/pkg/worker"
)

// TODO: これはcontrollerに移す
// サービスを別のモジュールから使う場合は、
// 直接このサービスを呼び出すのではなく、
// 一度ServiceAdapterを通して呼び出すこと
// serviceの返す値は必ずDTOにすること
// modelを返さないように実装すること
// modelはserviceの中で処理でのみ使う。
//
//ensoria:mock
type UserService interface {
	Something() string
	GetPostContent(postId string) (string, error)
	GetById(id int64) (*model.User, error)
}

// gRPCクライアントが必要な場合は、クライアントの型を指定する
// order serviceについては、すでにorderのモジュールでAsされているので、
// このmoduleの`init`でAsする必要はなく、dikitが自動的に解決してくれる
func NewUserService(
	repository repository.UserRepository,
	postClient pbPost.PostClient,
	orderService order.OrderService,
	jobQueue worker.Enqueuer,
) *userService {
	return &userService{
		repository:   repository,
		postClient:   postClient,
		orderService: orderService,
		jobQueue:     jobQueue,
	}
}

type userService struct {
	repository   repository.UserRepository
	postClient   pbPost.PostClient
	orderService order.OrderService
	jobQueue     worker.Enqueuer
}

func (s *userService) Something() string {
	fmt.Printf("injected orderService: %T\n", s.orderService)
	s.orderService.GetOrder()

	// worker test contextは基本的には`context.Background()`を使う
	// request.Context()などを使わないように注意すること
	a, err := s.jobQueue.Enqueue(context.Background(), "simple_log", map[string]any{
		"message": "UserServiceImpl.Something called",
	})
	if err != nil {
		fmt.Printf("failed to enqueue job: %v\n", err)
	} else {
		fmt.Printf("enqueued job: %v\n", a)
	}

	return "hoge"
}

func (s *userService) GetPostContent(postId string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p, err := s.postClient.GetPost(ctx, &pbPost.GetPostRequest{
		PostId: postId,
	})
	if err != nil {
		return "", err
	}
	return p.Content, nil

}

func (s *userService) GetById(id int64) (*model.User, error) {
	return s.repository.GetByID(id)
}
