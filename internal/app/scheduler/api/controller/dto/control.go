package dto

import "time"

type TaskStateResponse struct {
	TaskName   string     `json:"taskName"`
	Status     string     `json:"status"`
	PausedAt   *time.Time `json:"pausedAt,omitempty"`
	DisabledAt *time.Time `json:"disabledAt,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

type TaskControl struct {
	Message string `json:"message"`
}

type PauseTask struct {
	Reason string `json:"reason"`
}

type DisableTask struct {
	Reason string `json:"reason"`
}
