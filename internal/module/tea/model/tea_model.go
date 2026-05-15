package model

import "time"

type Tea struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	Price     int       `db:"price"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
