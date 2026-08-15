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
					red: "hsl(var(--surface-red))",
					purple: "hsl(var(--surface-purple))",
					magenta: "hsl(var(--surface-magenta))",
					"git-added": "hsl(var(--surface-git-added))",
					"git-deleted": "hsl(var(--surface-git-deleted))",
					"git-merged": "hsl(var(--surface-git-merged))",
				},
				border: {
					DEFAULT: "hsl(var(--border-default))",
					warning: "hsl(var(--border-warning))",
					green: "hsl(var(--border-green))",
					pending: "hsl(var(--border-sky))",
					destructive: "hsl(var(--border-destructive))",
					success: "hsl(var(--border-success))",
					secondary: "hsl(var(--border-secondary))",
					purple: "hsl(var(--border-purple))",
					magenta: "hsl(var(--border-magenta))",
				},
				overlay: "hsla(var(--overlay-default))",
				input: "hsl(var(--input))",
				ring: "hsl(var(--ring))",
				highlight: {
					purple: "hsl(var(--highlight-purple))",
					green: "hsl(var(--highlight-green))",
					orange: "hsl(var(--highlight-orange))",
					grey: "hsl(var(--highlight-grey))",
					sky: "hsl(var(--highlight-sky))",
					red: "hsl(var(--highlight-red))",
					magenta: "hsl(var(--highlight-magenta))",
				},
				syntax: {
					key: "hsl(var(--syntax-key))",
					string: "hsl(var(--syntax-string))",
					number: "hsl(var(--syntax-number))",
					boolean: "hsl(var(--syntax-boolean))",
				},
				git: {
					added: "hsl(var(--git-added))",
					deleted: "hsl(var(--git-deleted))",
					modified: "hsl(var(--git-modified))",
					merged: "hsl(var(--git-merged))",
					"added-bright": "hsl(var(--git-added-bright))",
					"deleted-bright": "hsl(var(--git-deleted-bright))",
					"merged-bright": "hsl(var(--git-merged-bright))",
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
				// Animates transform (compositor-friendly) instead of `left`,
				// which would trigger layout on every frame. 100cqw resolves
				// against the nearest inline-size container.
				"caret-scan": {
					"0%": { transform: "translateX(-100%)" },
					"100%": { transform: "translateX(100cqw)" },
				},
				// Sweeps the text highlight, then rests for the remainder of the
				// cycle. The hold between 60% and 100% produces identical
				// computed styles, letting the browser skip repaints while the
				// shimmer is "parked" between sweeps.
				shimmer: {
					"0%": { backgroundPosition: "100% center" },
					"60%": { backgroundPosition: "0% center" },
					"100%": { backgroundPosition: "0% center" },
				},
				"zip-right": {
					"0%": { left: "0%", width: "0%" },
					"30%": { left: "0%", width: "40%" },
					"100%": { left: "100%", width: "0%" },
				},
				// Matches MUI LinearProgress bar1/bar2 indeterminate motion; two
				// staggered bars are required so one is visible while the other
				// resets. Ported from MUI's left/right offsets to transform so
				// the animation runs on the compositor instead of triggering
				// layout on every frame. Bars are full-width with origin-left:
				// translateX positions the left edge, scaleX sets the visible
				// width fraction.
				"bar-indeterminate": {
					"0%": { transform: "translateX(-35%) scaleX(0.35)" },
					"60%": { transform: "translateX(100%) scaleX(0.9)" },
					"100%": { transform: "translateX(100%) scaleX(0.9)" },
				},
				"bar-indeterminate-2": {
					"0%": { transform: "translateX(-200%) scaleX(2)" },
					"60%": { transform: "translateX(107%) scaleX(0.01)" },
					"100%": { transform: "translateX(107%) scaleX(0.01)" },
				},
			},
			animation: {
				loading: "loading 2s ease-in-out infinite alternate",
				"caret-scan": "caret-scan 3s ease-in-out infinite",
				shimmer: "shimmer 3.5s linear infinite",
				// Discrete 8-step rotation for the segmented Spinner. Stepping
				// caps rendering at 10 updates/s instead of the display refresh
				// rate, and reads as intentional on 8-leaf spinner geometry.
				"spin-discrete": "spin 0.8s steps(8) infinite",
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
