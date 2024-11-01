const colors = require("tailwindcss/colors");

/** @type {import('tailwindcss').Config} */
module.exports = {
	corePlugins: {
		preflight: false,
	},
	darkMode: ["selector"],
	content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
	important: "#root",
	theme: {
		fontSize: {
			sm: ["14px", "24px"],
			"3xl": ["32px", "40px"],
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
					disabled: colors.zinc[600],
					invert: "var(--content-invert)",
					success: colors.green[600],
					danger: colors.red[500],
					link: colors.blue[500],
				},
				surface: {
					primary: colors.zinc[950],
					secondary: colors.zinc[900],
					tertiary: colors.zinc[800],
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
