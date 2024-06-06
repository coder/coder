-- This default is just a temporary thing to avoid null errors when first creating the column.
alter table organizations
	add column display_name text not null default '';

update organizations
	set display_name = name;

-- We can remove the default now that everything has been copied.
alter table organizations
	alter column display_name drop default;
