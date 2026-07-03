package dto

type Post struct {
	ID      uint   `db:"id"`
	UserID  uint   `db:"user_id"`
	Title   string `db:"title"`
	Content string `db:"content"`
}
