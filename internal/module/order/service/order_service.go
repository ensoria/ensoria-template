package service

import (
	"errors"

	"github.com/ensoria/ensoria-template/internal/module/order/dto"
	"github.com/ensoria/ensoria-template/internal/module/order/repository"
)

// ErrOrderNotFound means no order exists under that id.
//
// It is the service's own error rather than the repository's, so that a
// controller does not have to know which storage the order came from to tell
// "gone" from "the store did not answer".
var ErrOrderNotFound = errors.New("order: no such order")

// OrderService is what the rest of the application sees of orders.
//
// ⚠ Every method answers with a DTO, never a model. A model is the shape the
// storage holds and it changes with the storage; letting one out of this
// package would put every caller in the way of that change. Converting is
// dto.To*'s job — see dto.ToOrder.
//
//ensoria:mock
type OrderService interface {
	GetOrder() (*dto.Order, error)
	// FindOrder returns one order, or ErrOrderNotFound.
	FindOrder(id uint) (*dto.Order, error)
	// UpdateOrder replaces the order's mutable fields and returns the result.
	UpdateOrder(id uint, in *dto.UpdateOrder) (*dto.Order, error)
}

// IsOwner reports whether the order belongs to the caller identified by subject.
//
// It is the whole definition of "owner" for orders, and it lives here because
// that is a fact about orders rather than about authorization: the endpoint
// declaration in controller/http refers to it (restkit.NewResourceCheck), and
// the framework runs it, but neither gets to decide what it means.
//
// ⚠ It takes a subject rather than an authkit.Principal on purpose. Keeping the
// authorization framework out of this signature is what lets the rule be read,
// tested and reused without it; the endpoint declaration is the one place that
// knows a caller has a Subject.
func IsOwner(subject string, order *dto.Order) bool {
	return order != nil && subject != "" && order.OwnerSubject == subject
}

func NewOrderService(repo repository.OrderRepository) *orderService {
	return &orderService{repo: repo}
}

type orderService struct {
	repo repository.OrderRepository
}

func (s *orderService) GetOrder() (*dto.Order, error) {
	return dto.NewOrder(1, "alice", 100.0, "completed"), nil
}

func (s *orderService) FindOrder(id uint) (*dto.Order, error) {
	order, err := s.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return dto.ToOrder(order), nil
}

func (s *orderService) UpdateOrder(id uint, in *dto.UpdateOrder) (*dto.Order, error) {
	order, err := s.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if in.Amount != nil {
		order.Total = int(*in.Amount)
	}
	order.Status = in.Status

	if err := s.repo.Save(order); err != nil {
		return nil, err
	}
	return dto.ToOrder(order), nil
}
