package restkit

// httpError は HTTPError の既定実装。ハンドラは restkit.NewError で任意の
// ステータス/コード/メッセージのエラーを返せる。
type httpError struct {
	status  int
	code    string
	message string
}

func (e *httpError) Error() string { return e.message }
func (e *httpError) Status() int   { return e.status }
func (e *httpError) Code() string  { return e.code }

// NewError はハンドラから返す HTTPError を作る。
// status がそのままレスポンスのステータスになり、code/message は docai の
// エラーエンベロープ `{"error":{"code","message"}}` に反映される。
// これを実装しないただの error を返した場合は 500 internal_error に丸められる。
func NewError(status int, code, message string) HTTPError {
	return &httpError{status: status, code: code, message: message}
}
