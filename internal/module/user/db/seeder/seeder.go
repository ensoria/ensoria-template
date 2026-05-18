package seeder

import (
	"database/sql"
	"time"

	encliseeder "github.com/ensoria/encli/pkg/seeder"
	"github.com/ensoria/gofake/pkg/faker"

	"github.com/ensoria/ensoria-template/internal/module/user/model"
)

type UserSeeder struct{}

func (s *UserSeeder) TableName() string { return "users" }

// このサンプルでは、seedするデータ型は、`model.User`にしていますが、必ずしも`model`の型を指定する必要はありません。
// 例えば、`model.User`のフィールドが、個別に定義した構造体や、ネストした構造体の場合は、seedでは対応できません。
// 基本的には、seedできるフィールドは、プリミティブ型（string, int, time.Timeなど）や、sql.NullStringのようなNULL許容型になります。
// その場合の対応方法は主に2つあります。
//  1. seedするカラムだけを含む構造体を`model`とは別に定義し、それを`Seeder`の`T`型として使用する（最もシンプルな方法）。
//  2. カスタム型に`database/sql/driver`パッケージの`Valuer`インターフェース（`Value() (driver.Value, error)`メソッド）を実装する。
//     これにより、`model`の型をそのまま使いながら、seedの際にカスタム型の値を適切に変換することができる。
func (s *UserSeeder) Seed(f faker.Faker) []model.User {
	now := time.Now()
	nickName1 := sql.NullString{String: f.Person.FirstName(), Valid: true}
	birthDate1 := f.Rand.Time.Past()
	gender1 := f.Rand.Num.IntBt(1, 2)

	return []model.User{
		{
			Name:      f.Person.Name(),
			Nickname:  nickName1,
			Birthdate: &birthDate1,
			Gender:    &gender1,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{Name: f.Person.Name(), CreatedAt: now, UpdatedAt: now},
		{Name: f.Person.Name()}, // null以外のフィールドで、ゼロ値の場合は、カラムに設定された`DEFAULT`が適用される
	}
}

func init() {
	encliseeder.Add("user", &UserSeeder{})
}
