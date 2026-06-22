package middleware

import "github.com/ensoria/rest/pkg/rest"

func UserPostMiddleware(next rest.Handler) rest.Handler {
	return func(r *rest.Request) *rest.Response {
		// Do something before calling the next handler
		return next(r)
	}
}
