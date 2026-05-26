# Warden — Runbook

## Break-Glass Is Not Working

**Symptoms**: Break-glass invocation returns 403 or the action is still blocked.

**Diagnosis**:

1. Confirm the user has the `breakglass:use` permission:
   ```bash
   curl -s -H "Cookie: session=<tok>" https://warden.example.com/api/v1/internal/me \
     | jq '.permissions'
   ```

2. Check the reason string is at least 20 characters — this is enforced server-side.

3. Look at recent audit events for the actor:
   ```bash
   curl -s -H "Cookie: session=<tok>" \
     "https://warden.example.com/api/v1/internal/audit/events?actor=<email>&limit=20" \
     | jq '.events[] | {type, payload}'
   ```

4. Check if `breakglass_cooldown` PBAC policy is active and the user recently used break-glass:
   ```bash
   curl -s -H "Cookie: session=<tok>" \
     https://warden.example.com/api/v1/internal/admin/pbac \
     | jq '.policies[] | select(.name=="breakglass_cooldown")'
   ```

**Fix**: If the cooldown is blocking a genuine emergency, an administrator with `pbac:write` can temporarily disable the policy:
```bash
curl -s -X PUT -H "Cookie: session=<admin-tok>" \
  -H "Content-Type: application/json" \
  -d '{"enabled":false,"config":{}}' \
  https://warden.example.com/api/v1/internal/admin/pbac/breakglass_cooldown
```
Re-enable it after the emergency is resolved.

---

## Cascade Stuck in `in_progress`

**Symptoms**: A hold custodian's cascade state shows `in_progress` for more than 10 minutes.

**Diagnosis**:

1. Query cascade states for the hold:
   ```sql
   SELECT hold_id, custodian_id, provider, status, attempts, last_error, updated_at
   FROM cascade_state
   WHERE hold_id = '<hold-uuid>'
   ORDER BY updated_at DESC;
   ```

2. Check River job queue for stuck jobs:
   ```sql
   SELECT id, kind, state, attempt, attempted_at, errors
   FROM river_job
   WHERE state IN ('running', 'retryable', 'scheduled')
     AND kind IN ('cascade_place', 'cascade_remove')
   ORDER BY attempted_at DESC
   LIMIT 20;
   ```

3. Check Warden server logs for the plugin error:
   ```bash
   docker logs warden 2>&1 | grep -E 'cascade|plugin' | tail -50
   ```

**Fix**:

- If the external provider API is temporarily down, River will retry automatically with exponential backoff (max 25 attempts).
- If the job errored fatally (e.g., credential expired), update the instance credentials then manually re-queue:
  ```sql
  UPDATE river_job SET state = 'available', attempt = 0
  WHERE id = <job-id>;
  ```
- If no job exists at all (orphaned cascade_state), manually reset the state and trigger reconciliation:
  ```sql
  UPDATE cascade_state SET status = 'pending'
  WHERE hold_id = '<hold-uuid>' AND provider = '<plugin>';
  ```
  The `ReconcileHoldsJob` runs every 2 minutes and will re-enqueue the job.

---

## OIDC Login Fails

**Symptoms**: Clicking "Sign In" redirects to the OIDC provider but returns an error or loops back to the login page.

**Diagnosis**:

1. Confirm `OIDC_REDIRECT_URL` matches exactly what is registered in your identity provider (including `http` vs `https` and trailing slash).

2. Check server logs for OIDC errors:
   ```bash
   docker logs warden 2>&1 | grep -i oidc | tail -20
   ```

3. Test OIDC discovery endpoint:
   ```bash
   curl -s "$OIDC_ISSUER/.well-known/openid-configuration" | jq .
   ```

4. Confirm `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET` are correctly set (no trailing whitespace):
   ```bash
   docker exec warden printenv | grep OIDC
   ```

**Fix**:

- If the discovery endpoint is unreachable, check network connectivity from the container to the identity provider.
- If the redirect URI is wrong, update both the env var and the IdP's allowed redirect URIs.
- If clock skew is causing token validation failures, ensure the server's system clock is in sync (NTP).

---

## Database Full

**Symptoms**: PostgreSQL returns `no space left on device` errors; Warden API returns 500.

**Diagnosis**:

1. Check disk usage:
   ```bash
   df -h /var/lib/docker/volumes
   # or inside container:
   docker exec postgres df -h /var/lib/postgresql/data
   ```

2. Find the largest tables:
   ```sql
   SELECT relname, pg_size_pretty(pg_total_relation_size(oid))
   FROM pg_class
   WHERE relkind = 'r'
   ORDER BY pg_total_relation_size(oid) DESC
   LIMIT 10;
   ```

3. Check for bloated indexes (vacuuming lag):
   ```sql
   SELECT relname, n_dead_tup, last_autovacuum
   FROM pg_stat_user_tables
   ORDER BY n_dead_tup DESC;
   ```

**Fix**:

- Free disk space on the host or expand the volume.
- Run manual VACUUM to reclaim space:
  ```sql
  VACUUM ANALYZE;
  ```
- Archive old audit events if the `events` table is the culprit. Events are append-only; export then delete rows older than your retention window:
  ```sql
  DELETE FROM events WHERE created_at < now() - interval '1 year';
  VACUUM events;
  ```
- For Docker Compose, resize the volume by backing up and restoring to a new volume with more space.

---

## River Jobs Not Processing

**Symptoms**: Holds are stuck, actions are not completing, cascade states remain `pending`. River UI (if enabled) shows a growing queue.

**Diagnosis**:

1. Check if the River worker goroutines are running — look for "river" in server logs:
   ```bash
   docker logs warden 2>&1 | grep -i river | tail -20
   ```

2. Query job queue health:
   ```sql
   SELECT state, count(*) FROM river_job GROUP BY state;
   ```
   - `available`: queued, waiting for a worker
   - `running`: actively being processed
   - `retryable`: failed, will retry
   - `discarded`: permanently failed (max attempts exceeded)

3. Check for discarded jobs and their errors:
   ```sql
   SELECT kind, errors, finalized_at
   FROM river_job
   WHERE state = 'discarded'
   ORDER BY finalized_at DESC
   LIMIT 10;
   ```

4. Verify the Warden process has an active DB connection pool — look for `pgxpool` errors in logs.

**Fix**:

- If the server crashed and workers stopped, restart Warden:
  ```bash
  docker compose restart warden
  # or in k8s:
  kubectl rollout restart deployment/warden
  ```
- If jobs are `discarded` due to a configuration error (bad credentials, wrong plugin config), fix the configuration, then reset and re-queue:
  ```sql
  UPDATE river_job
  SET state = 'available', attempt = 0, errors = '[]'
  WHERE state = 'discarded' AND kind IN ('cascade_place', 'cascade_remove');
  ```
- If the DB connection is saturated, increase `max_connections` in PostgreSQL config or reduce the River worker concurrency by adjusting server configuration.
