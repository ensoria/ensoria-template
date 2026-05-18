package model

import "time"

type Order struct {
	ID            uint          `db:"id"`
	UserID        uint          `db:"user_id"`
	OrderDetailID uint          `db:"order_detail_id"`
	Total         int           `db:"total"`
	OrderDetails  []OrderDetail `db:"-"` // dbタグなし、または、"-"、"" の場合はseedから無視される
	CreatedAt     time.Time     `db:"created_at"`
	UpdatedAt     time.Time     `db:"updated_at"`
}
