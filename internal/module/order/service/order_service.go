package service

import (
	"fmt"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
)

//ensoria:mock
type OrderService interface {
	GetOrder() (*dto.Order, error)
}

func NewOrderService() *orderService {
	return &orderService{}
}

type orderService struct {
}

func (s *orderService) GetOrder() (*dto.Order, error) {
	fmt.Println("OrderServiceImpl GetOrder called")
	return &dto.Order{Id: 1, Amount: 100.0, Status: "completed"}, nil
}
