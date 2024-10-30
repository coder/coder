/** @type {import('tailwindcss').Config} */
module.exports = {
	corePlugins: {
		preflight: false,
	},
	content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
	important: "#root",
	theme: {
		extend: {
			borderRadius: {
				lg: "var(--radius)",
				md: "calc(var(--radius) - 2px)",
				sm: "calc(var(--radius) - 4px)",
			},
			colors: {},
		},
	},
	plugins: [require("tailwindcss-animate")],
};
