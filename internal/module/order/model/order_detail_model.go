package model

import "time"

type OrderDetail struct {
	ID          uint      `db:"id"`
	UserID      uint      `db:"user_id"`
	OrderID     uint      `db:"order_id"`
	ProductName string    `db:"product_name"`
	Price       int       `db:"price"`
	Quantity    int       `db:"quantity"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
