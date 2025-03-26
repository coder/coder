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
			size: {
				"icon-lg": "1.5rem",
				"icon-sm": "1.125rem",
				"icon-xs": "0.875rem",
			},
			fontSize: {
				"2xs": ["0.625rem", "0.875rem"],
				xs: ["0.75rem", "1.125rem"],
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
					link: "hsl(var(--content-link))",
					destructive: "hsl(var(--content-destructive))",
					warning: "hsl(var(--content-warning))",
				},
				surface: {
					primary: "hsl(var(--surface-primary))",
					secondary: "hsl(var(--surface-secondary))",
					tertiary: "hsl(var(--surface-tertiary))",
					quaternary: "hsl(var(--surface-quaternary))",
					invert: {
						primary: "hsl(var(--surface-invert-primary))",
						secondary: "hsl(var(--surface-invert-secondary))",
					},
					destructive: "hsl(var(--surface-destructive))",
					green: "hsl(var(--surface-green))",
					grey: "hsl(var(--surface-grey))",
					orange: "hsl(var(--surface-orange))",
					sky: "hsl(var(--surface-sky))",
				},
				border: {
					DEFAULT: "hsl(var(--border-default))",
					destructive: "hsl(var(--border-destructive))",
				},
				overlay: "hsla(var(--overlay-default))",
				input: "hsl(var(--input))",
				ring: "hsl(var(--ring))",
				highlight: {
					purple: "hsl(var(--highlight-purple))",
					green: "hsl(var(--highlight-green))",
					grey: "hsl(var(--highlight-grey))",
					sky: "hsl(var(--highlight-sky))",
				},
			},
			keyframes: {
				loading: {
					"0%": { opacity: 0.85 },
					"25%": { opacity: 0.7 },
					"50%": { opacity: 0.4 },
					"75%": { opacity: 0.3 },
					"100%": { opacity: 0.2 },
				},
			},
		},
	},
	plugins: [require("tailwindcss-animate"), require("@tailwindcss/typography")],
};
