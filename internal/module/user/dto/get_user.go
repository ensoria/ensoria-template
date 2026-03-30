package dto

import "github.com/ensoria/ensoria-template/internal/module/user/model"

type GetUser struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// dtoで、New***は、引数にフィールドを渡して作るもの
func NewGetUser(id int, name string) *GetUser {
	return &GetUser{
		Id:   id,
		Name: name,
	}
}

// dtoでは、To***も作ること
// To***は、そのドメインのmodelを必ず引数に取り、
// dtoに変換して返すもの
// 基本的には、service層からの戻り値に、modelからdtoに変換するために使う
func ToGetUser(m *model.User) *GetUser {
	return &GetUser{
		Id:   m.Id,
		Name: m.Name,
	}
}
