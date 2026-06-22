package http

import (
	"net/http"

	"github.com/ensoria/ensoria-template/internal/query/user_post/dto"
	"github.com/ensoria/ensoria-template/internal/query/user_post/service"
	"github.com/ensoria/rest/pkg/rest"
)

type Get struct {
	Service service.UserPostService
}

func NewGet(service service.UserPostService) *Get {
	return &Get{
		Service: service,
	}
}

func (c *Get) Handle(r *rest.Request) *rest.Response {

	user := c.Service.GetByID(1)

	return &rest.Response{
		Code: http.StatusOK,
		Body: &dto.GetUserPost{
			ID: user.ID,
		},
	}
}
