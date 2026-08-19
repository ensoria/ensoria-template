package middleware

import (
	"github.com/ensoria/rest/pkg/rest"
)

// For extra security.
func SysAdminOnly(next rest.Handler) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		// If necessary, add extra authentication logic here.
		return next(r)
	}
}
