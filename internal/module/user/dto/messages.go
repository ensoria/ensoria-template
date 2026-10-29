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

// UserWhoAmI asks the user WebSocket channel who the connection belongs to. It
// carries nothing: the caller is the connection's.
type UserWhoAmI struct{}

// UserWhoAmIReply says who the connection belongs to, as the server verified it.
type UserWhoAmIReply struct {
	// Subject identifies the caller.
	Subject string `json:"subject"`
	// Scheme is the kind of credential the connection was opened with.
	Scheme string `json:"scheme"`
}

// UserAnnounce is an announcement a client asks the user WebSocket channel to
// pass on to everyone connected to it.
type UserAnnounce struct {
	Message string `json:"message"`
}

// UserAnnouncement is the announcement as everyone connected receives it.
//
// It does not say who sent it: the channel admits anonymous listeners, and the
// sender's identity is not theirs to see.
type UserAnnouncement struct {
	Message string    `json:"message"`
	SentAt  time.Time `json:"sent_at"`
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
