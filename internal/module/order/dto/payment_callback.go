package dto

// PaymentCallback は決済事業者がサーバ間通信で送ってくる決済結果。
type PaymentCallback struct {
	PaymentId string `json:"paymentId"`
	OrderId   int    `json:"orderId"`
	Status    string `json:"status"`
}
