package dto

// PaymentCallback は決済事業者がサーバ間通信で送ってくる決済結果。
//
// OrderId がポインタなのは「未指定」と 0 を区別するため。JSON では欠けたキーが
// ゼロ値になるので、非ポインタの数値では指定されたかどうかを判定できない。
type PaymentCallback struct {
	PaymentId string `json:"paymentId"`
	OrderId   *int   `json:"orderId"`
	Status    string `json:"status"`
}
