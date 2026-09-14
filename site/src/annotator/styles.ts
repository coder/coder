// Mirrors the dashboard's dark theme tokens from site/src/index.css. The
// overlay renders inside a shadow root in a third-party page, so it cannot
// consume Tailwind or the theme CSS variables directly.
export const annotatorStyles = /* css */ `
:host {
	all: initial;
	--content-primary: hsl(0 0% 100%);
	--content-secondary: hsl(240 5% 65%);
	--content-disabled: hsl(240 5% 26%);
	--content-invert: hsl(240 10% 4%);
	--content-link: hsl(213 94% 68%);
	--content-destructive: hsl(0 91% 71%);
	--surface-primary: hsl(240 10% 4%);
	--surface-secondary: hsl(240 6% 10%);
	--surface-tertiary: hsl(240 4% 16%);
	--surface-invert-primary: hsl(240 6% 90%);
	--surface-invert-secondary: hsl(240 5% 65%);
	--surface-destructive: hsl(0 75% 15%);
	--border-default: hsl(240 4% 16%);
	--border-destructive: hsl(0 91% 71%);
	--radius-lg: 0.5rem;
	--radius-md: 0.375rem;
	--font-sans: "Geist Variable", system-ui, sans-serif;
	--font-mono: "Geist Mono Variable", ui-monospace, monospace;
	font-family: var(--font-sans);
	font-size: 0.75rem;
	line-height: 1rem;
	font-weight: 500;
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

button:focus-visible,
textarea:focus-visible {
	outline: none;
	box-shadow: 0 0 0 2px var(--content-link);
}

.toolbar {
	position: fixed;
	right: 16px;
	bottom: 16px;
	z-index: 2147483647;
	display: flex;
	align-items: center;
	gap: 4px;
	padding: 4px;
	background: var(--surface-primary);
	border: 1px solid var(--border-default);
	border-radius: var(--radius-lg);
	box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
}

.toolbar-button {
	display: inline-flex;
	align-items: center;
	gap: 4px;
	height: 32px;
	padding: 0 8px;
	border-radius: var(--radius-md);
	color: var(--content-secondary);
	white-space: nowrap;
	transition: color 150ms, background-color 150ms;
}

.toolbar-button:hover {
	color: var(--content-primary);
}

.toolbar-button[aria-pressed="true"] {
	background: var(--surface-tertiary);
	color: var(--content-primary);
}

.toolbar-button[disabled] {
	color: var(--content-disabled);
	pointer-events: none;
}

.toolbar-button svg {
	width: 1.125rem;
	height: 1.125rem;
	padding: 2px;
	flex: none;
}

.count {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	min-width: 16px;
	height: 16px;
	padding: 0 4px;
	border-radius: 999px;
	background: var(--surface-invert-primary);
	color: var(--content-invert);
	font-size: 0.625rem;
	line-height: 0.875rem;
	font-weight: 600;
}

.highlight {
	position: fixed;
	z-index: 2147483646;
	pointer-events: none;
	border: 1px solid var(--content-link);
	background: hsl(213 94% 68% / 0.12);
	border-radius: 2px;
	display: none;
}

.highlight-label {
	position: absolute;
	left: -1px;
	bottom: 100%;
	transform: translateY(-4px);
	padding: 1px 6px;
	border-radius: var(--radius-md);
	background: var(--surface-primary);
	border: 1px solid var(--border-default);
	color: var(--content-secondary);
	font-family: var(--font-mono);
	font-size: 0.625rem;
	line-height: 0.875rem;
	white-space: nowrap;
	max-width: 60vw;
	overflow: hidden;
	text-overflow: ellipsis;
}

.pin {
	position: fixed;
	z-index: 2147483646;
	display: flex;
	align-items: center;
	justify-content: center;
	width: 20px;
	height: 20px;
	border-radius: 999px;
	background: var(--surface-invert-primary);
	color: var(--content-invert);
	font-size: 0.625rem;
	font-weight: 600;
	box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
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
	border-color: hsl(142 71% 45% / 0.9);
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

.popup {
	position: fixed;
	z-index: 2147483647;
	width: 320px;
	padding: 12px;
	display: flex;
	flex-direction: column;
	gap: 8px;
	background: var(--surface-primary);
	border: 1px solid var(--border-default);
	border-radius: var(--radius-lg);
	box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
}

.popup-target {
	font-family: var(--font-mono);
	font-size: 0.625rem;
	line-height: 0.875rem;
	font-weight: 400;
	color: var(--content-secondary);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.popup textarea {
	font: inherit;
	font-weight: 400;
	font-size: 0.8125rem;
	line-height: 1.4;
	width: 100%;
	min-height: 64px;
	resize: vertical;
	padding: 8px;
	border-radius: var(--radius-md);
	border: 1px solid var(--border-default);
	background: var(--surface-secondary);
	color: var(--content-primary);
}

.popup textarea::placeholder {
	color: var(--content-secondary);
}

.popup-actions {
	display: flex;
	align-items: center;
	gap: 8px;
}

.popup-actions .spacer {
	flex: 1;
}

.button {
	height: 32px;
	padding: 0 8px;
	min-width: 64px;
	border-radius: var(--radius-md);
	border: 1px solid var(--border-default);
	background: transparent;
	color: var(--content-primary);
	transition: background-color 150ms;
}

.button:hover {
	background: var(--surface-secondary);
}

.button.primary {
	border-color: transparent;
	background: var(--surface-invert-primary);
	color: var(--content-invert);
	font-weight: 600;
}

.button.primary:hover {
	background: var(--surface-invert-secondary);
}

.button.danger {
	border-color: var(--border-destructive);
	background: var(--surface-destructive);
	font-weight: 600;
}

.button.danger:hover {
	background: transparent;
}

.hint {
	color: var(--content-secondary);
	font-weight: 400;
}
`;

export const pickingCursorStyles =
	"*, *::before, *::after { cursor: crosshair !important; }";
