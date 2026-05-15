package model

import "time"

type Order struct {
	ID            int       `db:"id"`
	UserID        int       `db:"user_id"`
	OrderDetailID int       `db:"order_detail_id"`
	Total         int       `db:"total"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}
