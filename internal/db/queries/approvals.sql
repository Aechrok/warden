-- name: CreateApproval :one
INSERT INTO approval_requests (
  requester_id,
  action_key,
  instance_id,
  target_email,
  params,
  reason,
  expires_at
)
VALUES ($1, $2, $3, $4, COALESCE($5, '{}'::jsonb), $6, $7)
RETURNING id, requester_id, action_key, instance_id, target_email, params, reason, status, reviewer_id, review_note, expires_at, reviewed_at, created_at;

-- name: GetApprovalByID :one
SELECT id, requester_id, action_key, instance_id, target_email, params, reason, status, reviewer_id, review_note, expires_at, reviewed_at, created_at
FROM approval_requests
WHERE id = $1;

-- name: ListPendingApprovals :many
SELECT id, requester_id, action_key, instance_id, target_email, params, reason, status, reviewer_id, review_note, expires_at, reviewed_at, created_at
FROM approval_requests
WHERE status = 'pending'
  AND expires_at > now()
ORDER BY created_at ASC
LIMIT $1 OFFSET $2;

-- name: UpdateApprovalStatus :one
UPDATE approval_requests
SET status      = $2,
    reviewer_id = $3,
    review_note = $4,
    reviewed_at = now()
WHERE id = $1
  AND status = 'pending'
RETURNING id, requester_id, action_key, instance_id, target_email, params, reason, status, reviewer_id, review_note, expires_at, reviewed_at, created_at;
