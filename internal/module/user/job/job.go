package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	appJob "github.com/ensoria/ensoria-template/internal/app/worker/job" // TODO: 名前を考え直す
	libWorker "github.com/ensoria/worker/pkg/job"
)

func NewUserJob(j *SimpleJob) *appJob.JobHandler {
	return &appJob.JobHandler{
		Name:    "simple_log",
		Handler: j.SimpleLogHandler,
		Options: &libWorker.Option{
			MaxRetries: 3,
			RetryDelay: 1 * time.Second,
			Timeout:    30 * time.Second,
		},
	}
}

type SimpleJob struct {
}

// Inject any dependencies if needed
func NewSimpleJob() *SimpleJob {
	return &SimpleJob{}
}

type SimpleLogPayload struct {
	Message string `json:"message"`
}

func (j *SimpleJob) SimpleLogHandler(ctx context.Context, payload []byte) error {
	var p SimpleLogPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("[SimpleLogJob] Processing message: %s", p.Message)

	// シミュレート処理時間
	time.Sleep(500 * time.Millisecond)

	log.Printf("[SimpleLogJob] Completed: %s", p.Message)
	return nil
}
