DROP TRIGGER IF EXISTS aibridge_interceptions_track_session ON aibridge_interceptions;

DROP FUNCTION IF EXISTS aibridge_session_track_interception();
DROP FUNCTION IF EXISTS aibridge_session_merge_value(text[], text);

DROP TABLE IF EXISTS aibridge_sessions;
