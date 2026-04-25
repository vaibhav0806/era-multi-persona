package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/vaibhav0806/era/internal/audit"
	"github.com/vaibhav0806/era/internal/queue"
)

// QueueAdapter adapts a *Docker to the queue.Runner interface.
type QueueAdapter struct {
	D       *Docker
	running *queue.RunningSet // optional; nil → no register/deregister
}

func (q *QueueAdapter) SetRunning(r *queue.RunningSet) { q.running = r }

func (q *QueueAdapter) Run(ctx context.Context, taskID int64, description, ghToken, repo string,
	maxIter, maxCents, maxWallSec int, onProgress ProgressCallback) (string, string, int64, int, []audit.Entry, error) {
	name := fmt.Sprintf("era-runner-%d-%d", taskID, time.Now().UnixNano())
	if q.running != nil {
		q.running.Register(taskID, name)
		defer q.running.Deregister(taskID)
	}
	out, err := q.D.Run(ctx, RunInput{
		TaskID:        taskID,
		Description:   description,
		GitHubToken:   ghToken,
		Repo:          repo,
		ContainerName: name,
		MaxIter:       maxIter,
		MaxCents:      maxCents,
		MaxWallSec:    maxWallSec,
	}, onProgress)
	if err != nil {
		return "", "", 0, 0, nil, err
	}
	return out.Branch, out.Summary, out.Tokens, out.CostCents, out.Audits, nil
}
