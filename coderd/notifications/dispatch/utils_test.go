package dispatch_test

func helpers() map[string]any {
	return map[string]any{
		"base_url":     func() string { return "http://test.com" },
		"current_year": func() string { return "2024" },
		"logo_url":     func() string { return "https://coder.com/coder-logo-horizontal.png" },
		"app_name":     func() string { return "Coder" },
	}
}
