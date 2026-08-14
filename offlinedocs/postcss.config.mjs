// PostCSS configuration for the offlinedocs build. It exists solely to run
// Tailwind CSS via @tailwindcss/postcss, which Fumadocs' UI and the Coder brand
// theme are built on; the app's global.css and brand.css are processed through
// this pipeline during `next build`.

const config = {
	plugins: {
		"@tailwindcss/postcss": {},
	},
};

export default config;
