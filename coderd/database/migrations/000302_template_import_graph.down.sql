-- Do the dance of dropping a view...
drop view template_version_with_user;


-- ...removing the column we added to the table...
alter table template_versions drop column cached_plan;


-- ...and finally recreating the view.
create view
	template_version_with_user
as
select
	template_versions.*,
	coalesce(visible_users.avatar_url, '') as created_by_avatar_url,
	coalesce(visible_users.username, '') as created_by_username
from
	template_versions
	left join
		visible_users
	on
		template_versions.created_by = visible_users.id;

comment on view template_version_with_user is 'Joins in the username + avatar url of the created by user.';
