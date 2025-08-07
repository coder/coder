-- New constraint: (deadline IS NOT zero AND deadline <= max_deadline) UNLESS max_deadline is zero.
-- Unfortunately, "zero" here means `time.Time{}`...

-- Update previous builds that would fail this new constraint. This matches the
-- intended behaviour of the autostop algorithm.
UPDATE
    workspace_builds
SET
    deadline = max_deadline
WHERE
    deadline > max_deadline
    AND max_deadline != '0001-01-01 00:00:00+00';

-- Add the new constraint.
ALTER TABLE workspace_builds
    ADD CONSTRAINT workspace_builds_deadline_below_max_deadline
        CHECK (
            (deadline != '0001-01-01 00:00:00+00'::timestamptz AND deadline <= max_deadline)
            OR max_deadline = '0001-01-01 00:00:00+00'::timestamptz
        );
