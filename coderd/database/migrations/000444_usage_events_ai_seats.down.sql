DROP INDEX IF EXISTS idx_usage_events_ai_seats;

-- Remove hb_ai_seats_v1 rows so the original constraint can be restored.
DELETE FROM usage_events WHERE event_type = 'hb_ai_seats_v1';
DELETE FROM usage_events_daily WHERE event_type = 'hb_ai_seats_v1';

-- Restore original constraint.
ALTER TABLE usage_events
  DROP CONSTRAINT usage_event_type_check,
  ADD CONSTRAINT usage_event_type_check CHECK (event_type IN ('dc_managed_agents_v1'));

-- Restore the original aggregate function without hb_ai_seats_v1 support.
CREATE OR REPLACE FUNCTION aggregate_usage_event()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.event_type NOT IN ('dc_managed_agents_v1') THEN
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
            WHEN NEW.event_type IN ('dc_managed_agents_v1') THEN
                jsonb_build_object(
                    'count',
                    COALESCE((usage_events_daily.usage_data->>'count')::bigint, 0) +
                    COALESCE((NEW.event_data->>'count')::bigint, 0)
                )
        END;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
