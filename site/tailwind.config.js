/** @type {import('tailwindcss').Config} */
module.exports = {
	corePlugins: {
		preflight: false,
	},
	darkMode: ["selector"],
	content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
	important: ["#root", "#storybook-root"],
	theme: {
		extend: {
			fontSize: {
				"2xs": ["0.626rem", "0.875rem"],
				sm: ["0.875rem", "1.5rem"],
				"3xl": ["2rem", "2.5rem"],
			},
			borderRadius: {
				lg: "var(--radius)",
				md: "calc(var(--radius) - 2px)",
				sm: "calc(var(--radius) - 4px)",
			},
			colors: {
				content: {
					primary: "hsl(var(--content-primary))",
					secondary: "hsl(var(--content-secondary))",
					disabled: "hsl(var(--content-disabled))",
					invert: "hsl(var(--content-invert))",
					success: "hsl(var(--content-success))",
					danger: "hsl(var(--content-danger))",
					link: "hsl(var(--content-link))",
				},
				surface: {
					primary: "hsl(var(--surface-primary))",
					secondary: "hsl(var(--surface-secondary))",
					tertiary: "hsl(var(--surface-tertiary))",
					invert: {
						primary: "hsl(var(--surface-invert-primary))",
						secondary: "hsl(var(--surface-invert-secondary))",
					},
					error: "hsl(var(--surface-error))",
				},
				border: {
					DEFAULT: "hsl(var(--border-default))",
					error: "hsl(var(--border-error))",
				},
				background: {
					DEFAULT: "hsl(var(--background))",
				},
			},
		},
	},
	plugins: [require("tailwindcss-animate")],
};
