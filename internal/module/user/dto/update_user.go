package dto

import "github.com/ensoria/validator/pkg/optional"

// UpdateUser はユーザーの部分更新(PATCH)のリクエストボディ。
//
// 各フィールドは optional.Optional なので、呼び出し元は3つのことができる:
//   - キーを送らない … そのフィールドには触れない
//   - null を送る    … そのフィールドを空にする
//   - 値を送る       … その値に変更する
//
// ポインタでは前2つを区別できない(どちらも nil になる)ため、部分更新では
// この型を使う。
type UpdateUser struct {
	// Name は表示名。空にはできない(NotNullIfSet で拒否する)。
	Name optional.Optional[string] `json:"name"`
	// Nickname は任意の呼び名。null を送ると消える。
	Nickname optional.Optional[string] `json:"nickname"`
}
