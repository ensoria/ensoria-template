package http_test

import (
	"context"
	"errors"

	"github.com/ensoria/worker/pkg/job"
	"github.com/ensoria/worker/pkg/queue"
	"github.com/ensoria/worker/pkg/worker"
)

// fakeQueue is an in-memory queue.Storage covering the calls these endpoints
// make. The interface is embedded so the rest of it does not have to be spelled
// out; a method the endpoints never reach panics rather than pretending to work.
type fakeQueue struct {
	queue.Storage

	queued     map[string]job.JobStatus
	deadLetter map[string]*job.JobData
	// failWith, when set, is returned by every call. It stands in for a queue
	// that cannot be reached.
	failWith error
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{
		queued:     map[string]job.JobStatus{},
		deadLetter: map[string]*job.JobData{},
	}
}

func (q *fakeQueue) GetStatus(_ context.Context, jobID string) (job.JobStatus, error) {
	if q.failWith != nil {
		return "", q.failWith
	}
	status, ok := q.queued[jobID]
	if !ok {
		return "", errors.New("job not found: " + jobID)
	}
	return status, nil
}

func (q *fakeQueue) Delete(_ context.Context, jobID string) error {
	if q.failWith != nil {
		return q.failWith
	}
	delete(q.queued, jobID)
	return nil
}

func (q *fakeQueue) Enqueue(_ context.Context, jd *job.JobData) error {
	if q.failWith != nil {
		return q.failWith
	}
	q.queued[jd.ID] = jd.Status
	return nil
}

func (q *fakeQueue) GetDeadLetterJob(_ context.Context, jobID string) (*job.JobData, error) {
	if q.failWith != nil {
		return nil, q.failWith
	}
	jd, ok := q.deadLetter[jobID]
	if !ok {
		return nil, errors.New("dead letter job not found: " + jobID)
	}
	return jd, nil
}

func (q *fakeQueue) GetDeadLetterJobs(_ context.Context, _ int) ([]*job.JobData, error) {
	if q.failWith != nil {
		return nil, q.failWith
	}
	jobs := make([]*job.JobData, 0, len(q.deadLetter))
	for _, jd := range q.deadLetter {
		jobs = append(jobs, jd)
	}
	return jobs, nil
}

func (q *fakeQueue) GetDeadLetterJobsByName(_ context.Context, name string, _ int) ([]*job.JobData, error) {
	if q.failWith != nil {
		return nil, q.failWith
	}
	var jobs []*job.JobData
	for _, jd := range q.deadLetter {
		if jd.Name == name {
			jobs = append(jobs, jd)
		}
	}
	return jobs, nil
}

func (q *fakeQueue) RemoveFromDeadLetter(_ context.Context, jobID string) error {
	if q.failWith != nil {
		return q.failWith
	}
	delete(q.deadLetter, jobID)
	return nil
}

// withDeadLetter records a job as having ended up in the dead letter queue.
func (q *fakeQueue) withDeadLetter(id, name string) *fakeQueue {
	q.deadLetter[id] = &job.JobData{ID: id, Name: name, Status: job.StatusFailed}
	return q
}

// withQueued records a job as waiting to run.
func (q *fakeQueue) withQueued(id string) *fakeQueue {
	q.queued[id] = job.StatusQueued
	return q
}

// workerWith builds a worker backed by the fake queue.
func workerWith(q queue.Storage) *worker.Worker { return worker.New(q) }
