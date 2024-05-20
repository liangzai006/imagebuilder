package core

import (
	"context"
	v1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type JobStatusQueue struct {
	Queue workqueue.RateLimitingInterface
	client.Client
}

func (q *JobStatusQueue) StartQueue(ctx context.Context) {

	for {
		item, shutdown := q.Queue.Get()
		if shutdown {
			return
		}
		job, isJob := item.(*v1.Job)
		if !isJob {
			q.Queue.Done(job)
			continue
		}
		q.listenJobStatus(ctx, job)
	}
}

func (q *JobStatusQueue) listenJobStatus(ctx context.Context, job *v1.Job) {
	newJob := v1.Job{}
	err := q.Get(ctx, client.ObjectKey{Name: job.Name, Namespace: job.Namespace}, &newJob)
	if err != nil {
		q.Queue.Done(job)
		return
	}

}
