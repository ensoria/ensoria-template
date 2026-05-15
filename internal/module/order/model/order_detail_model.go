package model

import "time"

type OrderDetail struct {
	Id          int       `db:"id"`
	UserId      int       `db:"user_id"`
	OrderId     int       `db:"order_id"`
	ProductName string    `db:"product_name"`
	Price       int       `db:"price"`
	Quantity    int       `db:"quantity"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
