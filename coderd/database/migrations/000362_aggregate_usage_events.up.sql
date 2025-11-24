CREATE TABLE usage_events_daily (
  day date NOT NULL, -- always grouped by day in UTC
  event_type text NOT NULL,
  usage_data jsonb NOT NULL,
  PRIMARY KEY (day, event_type)
);

COMMENT ON TABLE usage_events_daily IS 'usage_events_daily is a daily rollup of usage events. It stores the total usage for each event type by day.';
COMMENT ON COLUMN usage_events_daily.day IS 'The date of the summed usage events, always in UTC.';

-- Function to handle usage event aggregation
CREATE OR REPLACE FUNCTION aggregate_usage_event()
RETURNS TRIGGER AS $$
BEGIN
    -- Check for supported event types and throw error for unknown types
    IF NEW.event_type NOT IN ('dc_managed_agents_v1') THEN
        RAISE EXCEPTION 'Unhandled usage event type in aggregate_usage_event: %', NEW.event_type;
    END IF;

    INSERT INTO usage_events_daily (day, event_type, usage_data)
    VALUES (
        -- Extract the date from the created_at timestamp, always using UTC for
        -- consistency
        date_trunc('day', NEW.created_at AT TIME ZONE 'UTC')::date,
        NEW.event_type,
        NEW.event_data
    )
    ON CONFLICT (day, event_type) DO UPDATE SET
        usage_data = CASE
            -- Handle simple counter events by summing the count
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

-- Create trigger to automatically aggregate usage events
CREATE TRIGGER trigger_aggregate_usage_event
    AFTER INSERT ON usage_events
    FOR EACH ROW
    EXECUTE FUNCTION aggregate_usage_event();

-- Populate usage_events_daily with existing data
INSERT INTO
    usage_events_daily (day, event_type, usage_data)
SELECT
    date_trunc('day', created_at AT TIME ZONE 'UTC')::date AS day,
    event_type,
    jsonb_build_object('count', SUM((event_data->>'count')::bigint)) AS usage_data
FROM
    usage_events
WHERE
    -- The only event type we currently support is dc_managed_agents_v1
    event_type = 'dc_managed_agents_v1'
GROUP BY
    date_trunc('day', created_at AT TIME ZONE 'UTC')::date,
    event_type
ON CONFLICT (day, event_type) DO UPDATE SET
    usage_data = EXCLUDED.usage_data;
