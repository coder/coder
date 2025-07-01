-- All templates should opt out of dynamic parameters by default.
ALTER TABLE templates ALTER COLUMN use_classic_parameter_flow SET DEFAULT true;

UPDATE templates SET use_classic_parameter_flow = true
