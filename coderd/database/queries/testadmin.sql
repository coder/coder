-- name: DisableForeignKeys :exec
DO $$
DECLARE
    table_record record;
BEGIN
    FOR table_record IN 
        SELECT table_schema, table_name 
        FROM information_schema.tables 
        WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
        AND table_type = 'BASE TABLE'
    LOOP
        EXECUTE format('ALTER TABLE %I.%I DISABLE TRIGGER ALL', 
                    table_record.table_schema, 
                    table_record.table_name);
    END LOOP;
END;
$$;
