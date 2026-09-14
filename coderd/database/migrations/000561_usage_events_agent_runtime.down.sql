COMMENT ON COLUMN usage_events.created_at IS NULL;

DROP INDEX IF EXISTS idx_usage_events_agent_runtime;

-- Remove hb_agent_runtime_v1 rows so the previous constraint can be restored.
DELETE FROM usage_events WHERE event_type = 'hb_agent_runtime_v1';
DELETE FROM usage_events_daily WHERE event_type = 'hb_agent_runtime_v1';

ALTER TABLE usage_events
  DROP CONSTRAINT usage_event_type_check,
  ADD CONSTRAINT usage_event_type_check CHECK (event_type IN ('dc_managed_agents_v1', 'hb_ai_seats_v1'));

-- Restores the 000444 version of the function.
CREATE OR REPLACE FUNCTION aggregate_usage_event()
RETURNS TRIGGER AS $$
BEGIN
    -- Check for supported event types and throw error for unknown types.
    IF NEW.event_type NOT IN ('dc_managed_agents_v1', 'hb_ai_seats_v1') THEN
        RAISE EXCEPTION 'Unhandled usage event type in aggregate_usage_event: %', NEW.event_type;
    END IF;

    INSERT INTO usage_events_daily (day, event_type, usage_data)
    VALUES (
        date_trunc('day', NEW.created_at AT TIME ZONE 'UTC')::date,
        NEW.event_type,
        NEW.event_data
    )
    ON CONFLICT (day, event_type) DO UPDATE SET
        usage_data = CASE
            -- Handle simple counter events by summing the count.
            WHEN NEW.event_type IN ('dc_managed_agents_v1') THEN
                jsonb_build_object(
                    'count',
                    COALESCE((usage_events_daily.usage_data->>'count')::bigint, 0) +
                    COALESCE((NEW.event_data->>'count')::bigint, 0)
                )
			-- Heartbeat events: keep the max value seen that day
            WHEN NEW.event_type IN ('hb_ai_seats_v1') THEN
				jsonb_build_object(
					'count',
					GREATEST(
						COALESCE((usage_events_daily.usage_data->>'count')::bigint, 0),
						COALESCE((NEW.event_data->>'count')::bigint, 0)
					)
				)
        END;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
