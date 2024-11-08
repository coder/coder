/** @type {import('tailwindcss').Config} */
module.exports = {
	corePlugins: {
		preflight: false,
	},
	darkMode: ["selector"],
	content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
	important: ["#root", "#storybook-root"],
	theme: {
		fontSize: {
			"2xs": ["0.626rem","0.875rem"],
			sm: ["0.875rem", "1.5rem"],
			"3xl": ["2rem", "2.5rem"],
		},
		extend: {
			borderRadius: {
				lg: "var(--radius)",
				md: "calc(var(--radius) - 2px)",
				sm: "calc(var(--radius) - 4px)",
			},
			colors: {
				content: {
					primary: "var(--content-primary)",
					secondary: "var(--content-secondary)",
					disabled: "var(--content-disabled)",
					invert: "var(--content-invert)",
					success: "var(--content-success)",
					danger: "var(--content-danger)",
					link: "var(--content-link)",
				},
				surface: {
					primary: "var(--surface-primary)",
					secondary: "var(--surface-secondary)",
					tertiary: "var(--surface-tertiary)",
					invert: {
						primary: "var(--surface-invert-primary)",
						secondary: "var(--surface-invert-secondary)",
					},
					error: "var(--surface-error)",
				},
				border: {
					default: "var(--border-default)",
					error: "var(--border-error)",
				},
			},
		},
	},
	plugins: [require("tailwindcss-animate")],
};
