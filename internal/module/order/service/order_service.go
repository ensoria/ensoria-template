package service

import (
	"fmt"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
)

type OrderService interface {
	GetOrder() (*dto.Order, error)
}

func NewOrderService() OrderService {
	return &OrderServiceImpl{}
}

type OrderServiceImpl struct {
}

func (s *OrderServiceImpl) GetOrder() (*dto.Order, error) {
	fmt.Println("OrderServiceImpl GetOrder called")
	return &dto.Order{Id: 1, Amount: 100.0, Status: "completed"}, nil
}
