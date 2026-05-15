package model

import "time"

// Entityという名前だと、すべてのフィールドをValue Objectにするイメージがある
// そこまで厳密でないものにしたいため、modelにする

type User struct {
	ID        int       `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
