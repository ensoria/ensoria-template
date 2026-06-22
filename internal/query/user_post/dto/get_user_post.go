package dto

import "github.com/ensoria/ensoria-template/internal/query/user_post/record"

type GetUserPost struct {
	ID int64 `json:"id"`
}

// dtoで、New***は、引数にフィールドを渡して作るもの
func NewGetUserPost(id int64) *GetUserPost {
	return &GetUserPost{
		ID: id,
	}
}

func ToGetUserPost(m *record.UserPostRecord) *GetUserPost {
	return &GetUserPost{
		ID: m.ID,
	}
}
