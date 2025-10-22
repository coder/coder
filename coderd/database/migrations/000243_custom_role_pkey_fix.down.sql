ALTER TABLE custom_roles
	DROP CONSTRAINT custom_roles_unique_key;

ALTER TABLE custom_roles
	ADD CONSTRAINT custom_roles_pkey PRIMARY KEY (name);
