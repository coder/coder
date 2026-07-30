ALTER TABLE templates ALTER COLUMN use_classic_parameter_flow SET DEFAULT false;

UPDATE templates SET use_classic_parameter_flow = false
