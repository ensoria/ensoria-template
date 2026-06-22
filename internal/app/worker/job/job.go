package job

import libWorker "github.com/ensoria/worker/pkg/job"

type JobHandler struct {
	Name    string
	Handler libWorker.JobHandler
	Options *libWorker.Option
}
