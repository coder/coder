-- This is a deleted user that shares the same username and linked_id as the existing user below.
-- Any future migrations need to handle this case.
INSERT INTO public.users(id, email, username, hashed_password, created_at, updated_at, status, rbac_roles, deleted)
	VALUES ('a0061a8e-7db7-4585-838c-3116a003dd21', 'githubuser@coder.com', 'githubuser', '\x', '2022-11-02 13:05:21.445455+02', '2022-11-02 13:05:21.445455+02', 'active', '{}', true) ON CONFLICT DO NOTHING;
INSERT INTO public.organization_members VALUES ('a0061a8e-7db7-4585-838c-3116a003dd21', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', '2022-11-02 13:05:21.447595+02', '2022-11-02 13:05:21.447595+02', '{}') ON CONFLICT DO NOTHING;
INSERT INTO public.user_links(user_id, login_type, linked_id, oauth_access_token)
	VALUES('a0061a8e-7db7-4585-838c-3116a003dd21', 'github', '100', '');


INSERT INTO public.users(id, email, username, hashed_password, created_at, updated_at, status, rbac_roles, deleted)
	VALUES ('fc1511ef-4fcf-4a3b-98a1-8df64160e35a', 'githubuser@coder.com', 'githubuser', '\x', '2022-11-02 13:05:21.445455+02', '2022-11-02 13:05:21.445455+02', 'active', '{}', false) ON CONFLICT DO NOTHING;
INSERT INTO public.organization_members VALUES ('fc1511ef-4fcf-4a3b-98a1-8df64160e35a', 'bb640d07-ca8a-4869-b6bc-ae61ebb2fda1', '2022-11-02 13:05:21.447595+02', '2022-11-02 13:05:21.447595+02', '{}') ON CONFLICT DO NOTHING;
INSERT INTO public.user_links(user_id, login_type, linked_id, oauth_access_token)
	VALUES('fc1511ef-4fcf-4a3b-98a1-8df64160e35a', 'github', '100', '');

-- Additionally, there is no unique constraint on user_id. So also add another user_link for the same user.
-- This has happened on a production database.
INSERT INTO public.user_links(user_id, login_type, linked_id, oauth_access_token)
VALUES('fc1511ef-4fcf-4a3b-98a1-8df64160e35a', 'oidc', 'foo', '');
