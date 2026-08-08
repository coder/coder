ALTER TABLE ONLY organizations
	ADD CONSTRAINT organizations_name UNIQUE (name);
