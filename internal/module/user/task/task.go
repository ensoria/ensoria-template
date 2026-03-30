package task

import (
	"context"
	"log"

	schedTask "github.com/ensoria/ensoria-template/internal/app/scheduler/task"

	"github.com/ensoria/ensoria-template/internal/module/user/service"
	"github.com/ensoria/scheduler/pkg/cron"
)

func NewUserTask(task *SimpleTask) (*schedTask.ScheduledTask, error) {
	everyMinutes, err := cron.New("*", "*", "*", "*", "*")
	if err != nil {
		return nil, err
	}
	return &schedTask.ScheduledTask{
		Name: "SimpleUserTask",
		Cron: everyMinutes,
		Task: task.Run,
	}, nil
}

type SimpleTask struct {
	Service service.UserService
}

func NewSimpleTask(service service.UserService) *SimpleTask {
	return &SimpleTask{
		Service: service,
	}
}

func (t *SimpleTask) Run(ctx context.Context) error {
	log.Println("SimpleTask is running")
	return nil
}
