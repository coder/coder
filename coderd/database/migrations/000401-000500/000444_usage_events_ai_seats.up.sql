-- Expand the CHECK constraint to allow hb_ai_seats_v1.
ALTER TABLE usage_events
  DROP CONSTRAINT usage_event_type_check,
  ADD CONSTRAINT usage_event_type_check CHECK (event_type IN ('dc_managed_agents_v1', 'hb_ai_seats_v1'));

-- Partial index for efficient lookups of AI seat heartbeat events by time.
-- This will be used for the admin dashboard to see seat count over time.
CREATE INDEX idx_usage_events_ai_seats
  ON usage_events (event_type, created_at)
  WHERE event_type = 'hb_ai_seats_v1';

-- Update the aggregate function to handle hb_ai_seats_v1 events.
-- Heartbeat events replace the previous value for the same time period.
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
