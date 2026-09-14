CREATE TYPE provisioner_daemon_status AS ENUM ('offline', 'idle', 'busy');

COMMENT ON TYPE provisioner_daemon_status IS 'The status of a provisioner daemon.';
