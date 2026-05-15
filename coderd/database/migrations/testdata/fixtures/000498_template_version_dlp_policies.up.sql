INSERT INTO template_version_dlp_policies (
	id, template_version_id, name, ssh_access, web_terminal_access, port_forwarding_access, allowed_applications, display_name, created_at
) VALUES (
	'c1c1c1c1-0000-4000-8000-000000000001',
	'af58bd62-428c-4c33-849b-d43a3be07d93',
	'strict',
	true,
	false,
	true,
	ARRAY['code-server', 'vscode-desktop'],
	'Strict Policy',
	'2026-04-29 00:00:00.000000 +00:00'
);
