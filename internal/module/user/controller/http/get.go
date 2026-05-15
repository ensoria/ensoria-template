package http

import (
	"fmt"
	"net/http"

	"github.com/ensoria/ensoria-template/internal/module/user/dto"
	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/mb/pkg/mb"
	"github.com/ensoria/rest/pkg/rest"
)

// RESTでのresponse
// JSON or XML

type Get struct {
	Service service.UserService
	Publish mb.Publish
}

func NewGet(service service.UserService, publish mb.Publish) *Get {
	return &Get{
		Service: service,
		Publish: publish,
	}
}

func (c *Get) Handle(r *rest.Request) *rest.Response {

	fmt.Println("here? 1")
	c.Service.Something() // DEBUG:
	fmt.Println("here? 2")

	c.Publish("hello_world", []byte("Hello, World!"), map[string]string{"source": "Get.Handle"})

	// calling another modules gRPC service
	res, err := c.Service.GetPostContent("1")
	if err != nil {
		fmt.Println("gRPC call failed:", err)
	}
	fmt.Println("gRPC response received!", res)

	return &rest.Response{
		Xml:        true,
		Code:       http.StatusOK,
		AddHeaders: map[string]string{"Server": "net/http"},
		Body:       &dto.GetUser{ID: 1, Name: "hoge"},
	}
}
