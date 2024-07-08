update site_configs SET
	key = 'notification_banners',
	value = concat('[', value, ']')
where key = 'service_banner';
