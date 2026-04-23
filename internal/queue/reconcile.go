package queue

import (
	"context"
	"fmt"
)

// Reconcile sweeps any tasks still marked 'running' into 'failed' with a
// restart-lost-state reason, appending a `reconciled_failed` event per task.
// Call once at orchestrator startup before serving new requests.
// Returns number of tasks transitioned.
func (q *Queue) Reconcile(ctx context.Context) (int64, error) {
	ids, err := q.repo.ListRunningTaskIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list running: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	reason := "orchestrator restart, task state lost"
	n, err := q.repo.MarkRunningTasksFailed(ctx, reason)
	if err != nil {
		return 0, fmt.Errorf("mark failed: %w", err)
	}
	for _, id := range ids {
		_ = q.repo.AppendEvent(ctx, id, "reconciled_failed", quoteJSON(reason))
	}
	return n, nil
}
