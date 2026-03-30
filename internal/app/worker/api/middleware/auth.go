package middleware

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/app/http/dto"
	"github.com/ensoria/rest/pkg/rest"
)

// デフォルトでは、workerのAPIには誰もアクセスできない
// 各アプリケーションの実装で、このミドルウェアを上書きして、特定のクライアントからのみアクセスできるようにする
func SysAdminOnly(next rest.Handler) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		return &rest.Response{
			Code: http.StatusForbidden,
			Body: dto.Error{Message: "access denied"},
		}

		// return next(r)
	}
}
