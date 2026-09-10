package dto

import "github.com/ensoria/ensoria-template/internal/module/order/model"

// Order is the shape a caller sees.
type Order struct {
	Id int `json:"id"`
	// OwnerSubject is the caller the order belongs to.
	//
	// It is in the response because it is the input to the authorization rule
	// on PUT /order/{id}, and a caller who cannot see what the rule reads
	// cannot predict when they will be refused.
	//
	// ⚠ That is a choice this template makes, not one the framework makes.
	// Publishing an owner publishes who exists, so an API where subjects are
	// email addresses, or where reads are not already restricted to the owner,
	// should keep this out of the response and pass the rule a shape of its own
	// instead.
	OwnerSubject string  `json:"ownerSubject"`
	Amount       float64 `json:"amount"`
	Status       string  `json:"status"`
}

// dtoで、New***は、引数にフィールドを渡して作るもの
func NewOrder(id int, ownerSubject string, amount float64, status string) *Order {
	return &Order{Id: id, OwnerSubject: ownerSubject, Amount: amount, Status: status}
}

// dtoでは、To***も作ること
// To***は、そのドメインのmodelを必ず引数に取り、
// dtoに変換して返すもの
// 基本的には、service層からの戻り値に、modelからdtoに変換するために使う
func ToOrder(m *model.Order) *Order {
	if m == nil {
		return nil
	}
	return &Order{
		Id:           int(m.ID),
		OwnerSubject: m.OwnerSubject,
		Amount:       float64(m.Total),
		Status:       m.Status,
	}
}
