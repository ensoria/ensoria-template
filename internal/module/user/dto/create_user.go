package dto

import "github.com/ensoria/ensoria-template/internal/module/user/model"

type CreateUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// dtoで、New***は、引数にフィールドを渡して作るもの
func NewCreateUser(id int, name string) *CreateUser {
	return &CreateUser{
		ID:   id,
		Name: name,
	}
}

// dtoでは、To***も作ること
// To***は、そのドメインのmodelを必ず引数に取り、
// dtoに変換して返すもの
// 基本的には、service層からの戻り値に、modelからdtoに変換するために使う
func ToCreateUser(m *model.User) *CreateUser {
	return &CreateUser{
		ID:   m.ID,
		Name: m.Name,
	}
}
