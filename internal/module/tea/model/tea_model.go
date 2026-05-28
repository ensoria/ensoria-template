package model

import "time"

type Tea struct {
	ID         int       `db:"id"`
	Name       string    `db:"name"`
	Price      int       `db:"price" seed:"force"` // priceは0も有効な値なので、seed:"force" タグでゼロ値の挿入を強制する。
	BestBefore time.Time `db:"best_before"`
	// IsAvailableの型が*boolなのは、カラムがNULLABLEだからではなく、
	// ゼロ値のfalseをINSERTから除外してテーブル側のDEFAULT TRUEを優先させるため。
	IsAvailable *bool     `db:"is_available"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}
