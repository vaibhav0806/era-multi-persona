package runner

import "context"

// QueueAdapter adapts a *Docker to the queue.Runner interface, which uses
// plain (branch, summary, err) return values instead of a *RunOutput struct.
// The queue package imports runner, not the other way around, so the
// interface lives in queue and this adapter satisfies it.
type QueueAdapter struct {
	D *Docker
}

func (q QueueAdapter) Run(ctx context.Context, taskID int64, description string) (string, string, error) {
	out, err := q.D.Run(ctx, RunInput{TaskID: taskID, Description: description})
	if err != nil {
		return "", "", err
	}
	return out.Branch, out.Summary, nil
}
