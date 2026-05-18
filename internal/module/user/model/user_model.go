package model

import (
	"database/sql"
	"time"
)

// Entityという名前だと、すべてのフィールドをValue Objectにするイメージがある
// そこまで厳密でないものにしたいため、modelにする

type User struct {
	ID        uint           `db:"id"`
	Name      string         `db:"name"`
	Birthdate *time.Time     `db:"birthdate"` // NULL許容の例
	Gender    *int           `db:"gender"`    // NULL許容の例
	Nickname  sql.NullString `db:"nickname"`  // NULL許容の例。sql.NullStringを使用して、NULLと空文字を区別できるようにすることも可能
	CreatedAt time.Time      `db:"created_at"`
	UpdatedAt time.Time      `db:"updated_at"`
}
