package model

type Tea struct {
	Id    int    `db:"id"`
	Name  string `db:"name"`
	Price int    `db:"price"`
}
