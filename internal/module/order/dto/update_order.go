package dto

// UpdateOrder is the request body of PUT /order/{id}.
//
// PUT replaces the order, so every field it can set is required — a caller
// sending half of them is asking for the rest to be cleared, and that is
// almost never what they meant. Partial updates use PATCH and
// optional.Optional instead (see user.UpdateUser).
//
// Amount is a pointer so that a missing key and a deliberate 0 can be told
// apart: a JSON number that is absent decodes to the zero value, and a
// non-pointer field cannot report which of the two happened.
type UpdateOrder struct {
	// Amount is the order total.
	Amount *float64 `json:"amount"`
	// Status is where the order is in its lifecycle.
	Status string `json:"status"`
}
