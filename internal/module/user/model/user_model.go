package model

import "time"

// Entityという名前だと、すべてのフィールドをValue Objectにするイメージがある
// そこまで厳密でないものにしたいため、modelにする

type User struct {
	Id        int       `db:"id"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"create_at"`  // 適当な型
	UpdatedAt time.Time `db:"updated_at"` // 適当な型
}
