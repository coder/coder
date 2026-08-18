// PostCSS configuration for the offlinedocs build. It runs Tailwind CSS via
// @tailwindcss/postcss, which Fumadocs' UI and the Coder brand theme are built
// on, over the app's global.css and brand.css.

const config = {
	plugins: {
		"@tailwindcss/postcss": {},
	},
};

export default config;
