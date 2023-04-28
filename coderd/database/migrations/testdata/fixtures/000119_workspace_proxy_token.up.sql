INSERT INTO workspace_proxies
	(id, name, display_name, icon, url, wildcard_hostname, created_at, updated_at, deleted, token_hashed_secret)
VALUES
	(
		'cf8ede8c-ff47-441f-a738-d92e4e34a657',
		'us',
		'United States',
		'/emojis/us.png',
		'https://us.coder.com',
		'*.us.coder.com',
		'2023-03-30 12:00:00.000+02',
		'2023-03-30 12:00:00.000+02',
		false,
		'abc123'::bytea
	);
