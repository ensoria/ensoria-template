package model

import "time"

type Post struct {
	ID        uint      `db:"id"`
	UserID    uint      `db:"user_id"`
	Title     string    `db:"title"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
