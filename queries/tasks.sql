-- queries/tasks.sql

-- name: CreateTask :one
INSERT INTO tasks (description, status, target_repo, budget_profile, persona_name)
VALUES (?, 'queued', ?, ?, ?)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = ? LIMIT 1;

-- name: ListRecentTasks :many
SELECT * FROM tasks ORDER BY created_at DESC LIMIT ?;

-- name: ClaimNextQueuedTask :one
UPDATE tasks SET status = 'running', started_at = CURRENT_TIMESTAMP
WHERE id = (SELECT id FROM tasks WHERE status = 'queued' ORDER BY id ASC LIMIT 1)
RETURNING *;

-- name: MarkTaskCompleted :exec
UPDATE tasks SET
    status       = 'completed',
    branch_name  = ?,
    summary      = ?,
    tokens_used  = ?,
    cost_cents   = ?,
    finished_at  = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: MarkTaskFailed :exec
UPDATE tasks SET status = 'failed', error = ?, finished_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: AppendEvent :exec
INSERT INTO events (task_id, kind, payload) VALUES (?, ?, ?);

-- name: ListEventsForTask :many
SELECT * FROM events WHERE task_id = ? ORDER BY created_at ASC;

-- name: SetTaskStatus :exec
UPDATE tasks SET status = ? WHERE id = ?;

-- name: ListTasksBetween :many
SELECT * FROM tasks
WHERE created_at >= ? AND created_at < ?
ORDER BY id ASC;

-- name: SetPRNumber :exec
UPDATE tasks SET pr_number = ? WHERE id = ?;

-- name: ListRunningTaskIDs :many
SELECT id FROM tasks WHERE status='running';

-- name: MarkRunningTasksFailed :execrows
UPDATE tasks
   SET status='failed', error=?, finished_at=CURRENT_TIMESTAMP
 WHERE status='running';

-- name: SetBudgetProfile :exec
UPDATE tasks SET budget_profile = ? WHERE id = ?;

-- name: SetCompletionMessageID :exec
UPDATE tasks SET completion_message_id = ? WHERE id = ?;

-- name: GetTaskByCompletionMessageID :one
SELECT * FROM tasks WHERE completion_message_id = ? LIMIT 1;

-- name: CreateAskTask :one
INSERT INTO tasks (description, target_repo, budget_profile, read_only, status)
VALUES (?, ?, 'quick', 1, 'queued')
RETURNING *;

-- name: CountTasksByStatusSince :many
SELECT status, COUNT(*) AS count FROM tasks WHERE created_at >= ? GROUP BY status;

-- name: SumTokensSince :one
SELECT COALESCE(SUM(tokens_used), 0) AS total FROM tasks WHERE created_at >= ?;

-- name: SumCostCentsSince :one
SELECT COALESCE(SUM(cost_cents), 0) AS total FROM tasks WHERE created_at >= ?;

-- name: CountQueuedTasks :one
SELECT COUNT(*) AS count FROM tasks WHERE status = 'queued';
