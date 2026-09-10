// Package repository reads and writes orders.
//
// It is the layer that speaks in models: a repository answers with model.Order,
// and turning that into the shape a caller sees is the service's job (see
// service.OrderService). Keeping the boundary there is what lets the storage
// columns change without every controller changing with them.
package repository

import (
	"errors"
	"time"

	"github.com/ensoria/ensoria-template/internal/module/order/model"
)

// ErrOrderNotFound means no order exists under that id. It is the one benign
// outcome a lookup can report; anything else means the store could not be asked.
var ErrOrderNotFound = errors.New("order: no such order")

//ensoria:mock
type OrderRepository interface {
	// FindByID returns one order, or ErrOrderNotFound.
	FindByID(id uint) (*model.Order, error)
	// Save writes an order back.
	Save(order *model.Order) error
}

// orderRepository keeps orders in memory.
//
// A template has to run before a project has a database, so this stands in for
// one — replace it with a real implementation and nothing above it changes.
//
// ⚠ It holds orders belonging to *different* callers on purpose. An owner rule
// that is only ever tried against its own owner's order has not been tried.
type orderRepository struct {
	orders map[uint]*model.Order
}

func NewOrderRepository() *orderRepository {
	now := time.Now()
	return &orderRepository{orders: map[uint]*model.Order{
		1: {ID: 1, OwnerSubject: "alice", UserID: 1, Total: 1000, Status: "pending", CreatedAt: now, UpdatedAt: now},
		2: {ID: 2, OwnerSubject: "bob", UserID: 2, Total: 2000, Status: "pending", CreatedAt: now, UpdatedAt: now},
	}}
}

// FindByID returns a copy, so that whoever holds the result cannot edit the
// stored order by editing what they were handed. A repository backed by a
// database gives out copies whether it means to or not; one backed by a map has
// to say so.
func (r *orderRepository) FindByID(id uint) (*model.Order, error) {
	order, ok := r.orders[id]
	if !ok {
		return nil, ErrOrderNotFound
	}
	stored := *order
	return &stored, nil
}

func (r *orderRepository) Save(order *model.Order) error {
	if order == nil {
		return errors.New("order: nothing to save")
	}
	if _, ok := r.orders[order.ID]; !ok {
		return ErrOrderNotFound
	}
	saved := *order
	saved.UpdatedAt = time.Now()
	r.orders[order.ID] = &saved
	return nil
}
