package model

import "time"

type Order struct {
	ID uint `db:"id"`
	// OwnerSubject identifies the caller the order belongs to.
	//
	// It holds a subject — the same value a verified caller carries as
	// authkit.Principal.Subject — rather than a row id, because that is what an
	// authorization rule has to compare against. A numeric user id would have to
	// be translated on every check, and the identity provider is under no
	// obligation to issue subjects that look like row ids.
	OwnerSubject  string        `db:"owner_subject"`
	UserID        uint          `db:"user_id"`
	OrderDetailID uint          `db:"order_detail_id"`
	Total         int           `db:"total"`
	Status        string        `db:"status"`
	OrderDetails  []OrderDetail `db:"-"` // dbタグなし、または、"-"、"" の場合はseedから無視される
	CreatedAt     time.Time     `db:"created_at"`
	UpdatedAt     time.Time     `db:"updated_at"`
}
