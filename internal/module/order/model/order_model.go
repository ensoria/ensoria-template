package model

import "time"

type Order struct {
	Id            int       `db:"id"`
	UserId        int       `db:"user_id"`
	OrderDetailId int       `db:"order_detail_id"`
	Total         int       `db:"total"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}
