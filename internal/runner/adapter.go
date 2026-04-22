package runner

import "context"

// QueueAdapter adapts a *Docker to the queue.Runner interface.
type QueueAdapter struct {
	D *Docker
}

func (q QueueAdapter) Run(ctx context.Context, taskID int64, description string) (string, string, int64, int, error) {
	out, err := q.D.Run(ctx, RunInput{TaskID: taskID, Description: description})
	if err != nil {
		return "", "", 0, 0, err
	}
	return out.Branch, out.Summary, out.Tokens, out.CostCents, nil
}
