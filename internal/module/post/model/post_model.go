package model

type Post struct {
	Id      int    `db:"id"`
	UserId  int    `db:"user_id"`
	Title   string `db:"title"`
	Content string `db:"content"`
}
