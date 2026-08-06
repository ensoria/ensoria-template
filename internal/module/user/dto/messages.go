package dto

import "time"

// HelloWorld is the payload of the `hello_world` channel, which the template
// uses to demonstrate a subscription and a publication on the same channel.
type HelloWorld struct {
	Message string `json:"message"`
	Source  string `json:"source,omitempty"`
}

// UserEcho is the payload a client sends on the user WebSocket channel.
type UserEcho struct {
	Message string `json:"message"`
}

// UserEchoReply is the payload the server pushes back for each UserEcho.
type UserEchoReply struct {
	Message    string    `json:"message"`
	ReceivedAt time.Time `json:"received_at"`
}

// UserCreated announces that a user was created.
//
// It is published for consumers outside this application, which is what makes it
// part of the public contract rather than an internal detail: once another
// service subscribes to it, the shape of this struct is a promise.
type UserCreated struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}
