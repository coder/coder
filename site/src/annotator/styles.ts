// Mirrors the dashboard's dark theme tokens and component primitives from
// site/src/index.css and site/src/components (Button, Badge, Tooltip,
// Popover, Textarea). The overlay renders inside a shadow root in a
// third-party page, so it cannot consume Tailwind or the theme variables
// directly; each rule below names the Tailwind classes it reproduces.
export const annotatorStyles = /* css */ `
:host {
	all: initial;
	--content-primary: hsl(0 0% 100%);
	--content-secondary: hsl(240 5% 65%);
	--content-disabled: hsl(240 5% 26%);
	--content-invert: hsl(240 10% 4%);
	--content-link: hsl(213 94% 68%);
	--surface-primary: hsl(240 10% 4%);
	--surface-secondary: hsl(240 6% 10%);
	--surface-tertiary: hsl(240 4% 16%);
	--surface-invert-primary: hsl(240 6% 90%);
	--surface-invert-secondary: hsl(240 5% 65%);
	--surface-destructive: hsl(0 75% 15%);
	--surface-red: hsl(0 75% 15%);
	--highlight-red: hsl(0 91% 71%);
	--highlight-green: hsl(142 71% 45%);
	--border: hsl(240 4% 16%);
	--border-destructive: hsl(0 91% 71%);
	--radius-md: 0.375rem;
	--shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
	--shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
	--font-sans: "Geist Variable", system-ui, sans-serif;
	--font-mono: "Geist Mono Variable", ui-monospace, monospace;
	font-family: var(--font-sans);
	color: var(--content-primary);
}

* {
	box-sizing: border-box;
}

button {
	font: inherit;
	color: inherit;
	background: none;
	border: 0;
	padding: 0;
	margin: 0;
	cursor: pointer;
}

/* focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link */
button:focus-visible,
textarea:focus-visible {
	outline: none;
	box-shadow: 0 0 0 2px var(--content-link);
}

/* Popover surface: rounded-md border bg-surface-primary shadow-md, p-1 */
.toolbar {
	position: fixed;
	right: 16px;
	bottom: 16px;
	z-index: 2147483647;
	display: flex;
	align-items: center;
	gap: 2px;
	padding: 4px;
	background: var(--surface-primary);
	border: 1px solid var(--border);
	border-radius: var(--radius-md);
	box-shadow: var(--shadow-md);
}

/* Button variant=subtle size=icon: size-8 rounded-md text-content-secondary
   hover:text-content-primary [&>svg]:size-icon-sm [&>svg]:p-0.5 */
.icon-button {
	position: relative;
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 2rem;
	height: 2rem;
	padding: 0 0.375rem;
	border-radius: var(--radius-md);
	color: var(--content-secondary);
	transition: color 150ms, background-color 150ms;
}

.icon-button:hover {
	color: var(--content-primary);
}

.icon-button[aria-pressed="true"] {
	background: var(--surface-tertiary);
	color: var(--content-primary);
}

/* disabled:pointer-events-none disabled:text-content-disabled */
.icon-button[disabled] {
	pointer-events: none;
	color: var(--content-disabled);
}

.icon-button svg {
	width: 1.125rem;
	height: 1.125rem;
	padding: 0.125rem;
	flex: none;
	pointer-events: none;
}

/* Tooltip: rounded-md bg-surface-primary px-3 py-2 text-xs font-medium
   text-content-secondary border border-border */
.icon-button[data-tip]:hover::after {
	content: attr(data-tip);
	position: absolute;
	bottom: calc(100% + 6px);
	right: 0;
	padding: 0.5rem 0.75rem;
	border-radius: var(--radius-md);
	background: var(--surface-primary);
	border: 1px solid var(--border);
	color: var(--content-secondary);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 500;
	white-space: nowrap;
	pointer-events: none;
}

/* Badge variant=default: rounded-md border border-surface-secondary
   bg-surface-secondary text-content-secondary px-1.5 py-0.5 text-xs shadow-sm */
.badge {
	display: inline-flex;
	align-items: center;
	padding: 0.125rem 0.375rem;
	margin: 0 0.25rem;
	border-radius: var(--radius-md);
	border: 1px solid var(--surface-secondary);
	background: var(--surface-secondary);
	color: var(--content-secondary);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 500;
	box-shadow: var(--shadow-sm);
}

.badge[hidden] {
	display: none;
}

.highlight {
	position: fixed;
	z-index: 2147483646;
	pointer-events: none;
	border: 1px solid var(--content-link);
	background: hsl(213 94% 68% / 0.1);
	border-radius: 2px;
	display: none;
}

/* Same surface as Tooltip, mono text. */
.highlight-label {
	position: absolute;
	left: -1px;
	bottom: 100%;
	transform: translateY(-6px);
	padding: 0.25rem 0.5rem;
	border-radius: var(--radius-md);
	background: var(--surface-primary);
	border: 1px solid var(--border);
	color: var(--content-secondary);
	font-family: var(--font-mono);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 500;
	white-space: nowrap;
	max-width: 60vw;
	overflow: hidden;
	text-overflow: ellipsis;
}

/* Numbered pin: Badge shape in the inverted button colors. */
.pin {
	position: fixed;
	z-index: 2147483646;
	display: flex;
	align-items: center;
	justify-content: center;
	min-width: 1.25rem;
	height: 1.25rem;
	padding: 0 0.25rem;
	border-radius: var(--radius-md);
	background: var(--surface-invert-primary);
	color: var(--content-invert);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 600;
	box-shadow: var(--shadow-sm);
	transform: translate(-50%, -50%);
}

.pin:hover {
	background: var(--surface-invert-secondary);
}

.shimmer {
	position: fixed;
	z-index: 2147483645;
	pointer-events: none;
	border-radius: 4px;
	border: 1px solid hsl(213 94% 68% / 0.6);
	background: linear-gradient(
		100deg,
		hsl(213 94% 68% / 0.08) 20%,
		hsl(213 94% 68% / 0.28) 50%,
		hsl(213 94% 68% / 0.08) 80%
	);
	background-size: 250% 100%;
	animation: coder-shimmer 1.6s ease-in-out infinite;
}

.shimmer.done {
	animation: coder-done 1.2s ease-out forwards;
	background: hsl(142 71% 45% / 0.18);
	border-color: var(--highlight-green);
}

@keyframes coder-shimmer {
	0% {
		background-position: 100% 0;
	}
	100% {
		background-position: -100% 0;
	}
}

@keyframes coder-done {
	0% {
		opacity: 1;
	}
	70% {
		opacity: 1;
	}
	100% {
		opacity: 0;
	}
}

/* PopoverContent: w-72 rounded-md border bg-surface-primary shadow-md, p-3 */
.popup {
	position: fixed;
	z-index: 2147483647;
	width: 18rem;
	padding: 0.75rem;
	display: flex;
	flex-direction: column;
	gap: 0.5rem;
	background: var(--surface-primary);
	border: 1px solid var(--border);
	border-radius: var(--radius-md);
	box-shadow: var(--shadow-md);
}

/* text-xs font-mono text-content-secondary */
.popup-target {
	font-family: var(--font-mono);
	font-size: 0.75rem;
	line-height: 1rem;
	color: var(--content-secondary);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

/* Textarea: min-h-[60px] w-full px-3 py-2 text-sm rounded-md border
   border-border bg-transparent shadow-xs placeholder:text-content-secondary */
.popup textarea {
	font-family: var(--font-sans);
	font-size: 0.875rem;
	line-height: 1.5rem;
	font-weight: 500;
	width: 100%;
	min-height: 60px;
	resize: vertical;
	padding: 0.5rem 0.75rem;
	border-radius: var(--radius-md);
	border: 1px solid var(--border);
	background: transparent;
	color: var(--content-primary);
	box-shadow: 0 1px 2px 0 rgb(0 0 0 / 0.05);
}

.popup textarea::placeholder {
	color: var(--content-secondary);
}

.popup-actions {
	display: flex;
	align-items: center;
	gap: 0.5rem;
}

.popup-actions .spacer {
	flex: 1;
}

/* Button size=sm: min-w-20 h-8 px-2 text-xs rounded-md font-medium.
   variant=default: bg-surface-invert-primary font-semibold text-content-invert
   hover:bg-surface-invert-secondary */
.button {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	min-width: 5rem;
	height: 2rem;
	padding: 0 0.5rem;
	border-radius: var(--radius-md);
	border: 1px solid transparent;
	background: var(--surface-invert-primary);
	color: var(--content-invert);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 600;
	transition: background-color 150ms;
}

.button:hover {
	background: var(--surface-invert-secondary);
}

/* variant=destructive: border border-border-destructive font-semibold
   text-content-primary bg-surface-destructive hover:bg-transparent */
.button.destructive {
	border-color: var(--border-destructive);
	background: var(--surface-destructive);
	color: var(--content-primary);
}

.button.destructive:hover {
	background: transparent;
}

/* text-xs text-content-secondary */
.hint {
	color: var(--content-secondary);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 500;
}
`;

export const pickingCursorStyles =
	"*, *::before, *::after { cursor: crosshair !important; }";
