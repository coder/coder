/** @type {import('tailwindcss').Config} */
module.exports = {
	corePlugins: {
		preflight: false,
	},
	darkMode: ["selector"],
	content: [
		"./index.html",
		"./src/**/*.{js,ts,jsx,tsx}",
		"./node_modules/streamdown/dist/**/*.js",
		"./node_modules/@streamdown/*/dist/**/*.js",
	],
	important: ["#root", "#storybook-root"],
	theme: {
		extend: {
			fontFamily: {
				sans: `"Geist Variable", system-ui, sans-serif`,
				// `monospace, monospace` resets the font-size to 16px with the fallback.
				mono: `"Geist Mono Variable", monospace, monospace`,
			},
			// Tailwind v4 compatibility shims. v4 rebased a few defaults; these
			// keep the v3 rendering so the migration is visually a no-op. They can
			// be removed once usages move to native v4 utilities.
			// - ring: v4's bare `ring` is 1px; v3 (and our components) expect 3px.
			// - drop-shadow-md: v4 changed the value; restore the v3 stack.
			ringWidth: {
				DEFAULT: "3px",
			},
			dropShadow: {
				md: ["0 4px 3px rgb(0 0 0 / 0.07)", "0 2px 2px rgb(0 0 0 / 0.06)"],
			},
			size: {
				"icon-lg": "1.5rem",
				"icon-sm": "1.125rem",
				"icon-xs": "0.875rem",
			},
			fontSize: {
				"2xs": ["0.625rem", "0.875rem"],
				xs: ["0.75rem", { lineHeight: "1rem", fontWeight: "500" }],
				sm: ["0.875rem", { lineHeight: "1.5rem", fontWeight: "500" }],
				base: ["1rem", { lineHeight: "1.5rem", fontWeight: "400" }],
				"3xl": ["2rem", "2.5rem"],
			},
			borderRadius: {
				lg: "var(--radius)",
				md: "calc(var(--radius) - 2px)",
				sm: "calc(var(--radius) - 4px)",
				xs: "calc(var(--radius) - 6px)",
			},
			colors: {
				content: {
					primary: "hsl(var(--content-primary) / <alpha-value>)",
					secondary: "hsl(var(--content-secondary) / <alpha-value>)",
					disabled: "hsl(var(--content-disabled) / <alpha-value>)",
					invert: "hsl(var(--content-invert) / <alpha-value>)",
					success: "hsl(var(--content-success) / <alpha-value>)",
					link: "hsl(var(--content-link) / <alpha-value>)",
					destructive: "hsl(var(--content-destructive) / <alpha-value>)",
					warning: "hsl(var(--content-warning) / <alpha-value>)",
				},
				surface: {
					primary: "hsl(var(--surface-primary) / <alpha-value>)",
					secondary: "hsl(var(--surface-secondary) / <alpha-value>)",
					tertiary: "hsl(var(--surface-tertiary) / <alpha-value>)",
					quaternary: "hsl(var(--surface-quaternary) / <alpha-value>)",
					invert: {
						primary: "hsl(var(--surface-invert-primary) / <alpha-value>)",
						secondary: "hsl(var(--surface-invert-secondary) / <alpha-value>)",
					},
					destructive: "hsl(var(--surface-destructive) / <alpha-value>)",
					green: "hsl(var(--surface-green) / <alpha-value>)",
					grey: "hsl(var(--surface-grey) / <alpha-value>)",
					orange: "hsl(var(--surface-orange) / <alpha-value>)",
					sky: "hsl(var(--surface-sky) / <alpha-value>)",
					red: "hsl(var(--surface-red) / <alpha-value>)",
					purple: "hsl(var(--surface-purple) / <alpha-value>)",
					magenta: "hsl(var(--surface-magenta) / <alpha-value>)",
					"git-added": "hsl(var(--surface-git-added) / <alpha-value>)",
					"git-deleted": "hsl(var(--surface-git-deleted) / <alpha-value>)",
					"git-merged": "hsl(var(--surface-git-merged) / <alpha-value>)",
				},
				border: {
					DEFAULT: "hsl(var(--border-default) / <alpha-value>)",
					warning: "hsl(var(--border-warning) / <alpha-value>)",
					green: "hsl(var(--border-green) / <alpha-value>)",
					pending: "hsl(var(--border-sky) / <alpha-value>)",
					destructive: "hsl(var(--border-destructive) / <alpha-value>)",
					success: "hsl(var(--border-success) / <alpha-value>)",
					secondary: "hsl(var(--border-secondary) / <alpha-value>)",
					purple: "hsl(var(--border-purple) / <alpha-value>)",
					magenta: "hsl(var(--border-magenta) / <alpha-value>)",
				},
				overlay: "hsla(var(--overlay-default))",
				input: "hsl(var(--input) / <alpha-value>)",
				ring: "hsl(var(--ring) / <alpha-value>)",
				highlight: {
					purple: "hsl(var(--highlight-purple) / <alpha-value>)",
					green: "hsl(var(--highlight-green) / <alpha-value>)",
					orange: "hsl(var(--highlight-orange) / <alpha-value>)",
					grey: "hsl(var(--highlight-grey) / <alpha-value>)",
					sky: "hsl(var(--highlight-sky) / <alpha-value>)",
					red: "hsl(var(--highlight-red) / <alpha-value>)",
					magenta: "hsl(var(--highlight-magenta) / <alpha-value>)",
				},
				syntax: {
					key: "hsl(var(--syntax-key) / <alpha-value>)",
					string: "hsl(var(--syntax-string) / <alpha-value>)",
					number: "hsl(var(--syntax-number) / <alpha-value>)",
					boolean: "hsl(var(--syntax-boolean) / <alpha-value>)",
				},
				git: {
					added: "hsl(var(--git-added) / <alpha-value>)",
					deleted: "hsl(var(--git-deleted) / <alpha-value>)",
					modified: "hsl(var(--git-modified) / <alpha-value>)",
					merged: "hsl(var(--git-merged) / <alpha-value>)",
					"added-bright": "hsl(var(--git-added-bright) / <alpha-value>)",
					"deleted-bright": "hsl(var(--git-deleted-bright) / <alpha-value>)",
					"merged-bright": "hsl(var(--git-merged-bright) / <alpha-value>)",
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
				"caret-scan": {
					"0%": { left: "0%" },
					"100%": { left: "100%" },
				},
				"zip-right": {
					"0%": { left: "0%", width: "0%" },
					"30%": { left: "0%", width: "40%" },
					"100%": { left: "100%", width: "0%" },
				},
				// Matches MUI LinearProgress bar1/bar2 indeterminate keyframes; two
				// staggered bars are required so one is visible while the other resets.
				"bar-indeterminate": {
					"0%": {
						left: "-35%",
						right: "100%",
					},
					"60%": {
						left: "100%",
						right: "-90%",
					},
					"100%": {
						left: "100%",
						right: "-90%",
					},
				},
				"bar-indeterminate-2": {
					"0%": {
						left: "-200%",
						right: "100%",
					},
					"60%": {
						left: "107%",
						right: "-8%",
					},
					"100%": {
						left: "107%",
						right: "-8%",
					},
				},
			},
			animation: {
				loading: "loading 2s ease-in-out infinite alternate",
				"caret-scan": "caret-scan 3s ease-in-out infinite",
				"spin-once": "spin 1s cubic-bezier(0.4, 0, 0.2, 1)",
				"zip-right": "zip-right 1s cubic-bezier(0.4, 0, 0.2, 1)",
				"bar-indeterminate":
					"bar-indeterminate 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite",
				"bar-indeterminate-2":
					"bar-indeterminate-2 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite",
			},
		},
	},
	plugins: [require("tailwindcss-animate"), require("@tailwindcss/typography")],
};
