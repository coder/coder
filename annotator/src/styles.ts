// Mirrors the dashboard's dark theme tokens and component primitives
// (Button, Badge, Tooltip, Popover, Textarea) from site/src/index.css and
// site/src/components. The overlay renders inside a shadow root in a
// third-party page, so it cannot consume Tailwind or the theme variables
// directly.
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
	--border: hsl(240 4% 16%);
	--radius-md: 0.375rem;
	--radius-lg: 0.5rem;
	/* Shared by the selection outline and the working-state ring so the
	   two states read as the same box. */
	--outline-width: 2px;
	--shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1), 0 2px 4px -2px rgb(0 0 0 / 0.1);
	--shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1);
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

.icon-button[disabled] {
	pointer-events: none;
	color: var(--content-disabled);
}

/* Button variant=default: inverted surface, disabled falls back to
   surface-secondary like the dashboard. */
.icon-button.primary {
	background: var(--surface-invert-primary);
	color: var(--content-invert);
}

.icon-button.primary:hover {
	background: var(--surface-invert-secondary);
	color: var(--content-invert);
}

.icon-button.primary[disabled] {
	background: var(--surface-secondary);
	color: var(--content-disabled);
}

.icon-button svg {
	width: 1.125rem;
	height: 1.125rem;
	padding: 0.125rem;
	flex: none;
	pointer-events: none;
}

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


.highlight {
	position: fixed;
	z-index: 2147483646;
	pointer-events: none;
	border: var(--outline-width) dashed var(--content-link);
	border-radius: var(--radius-lg);
	display: none;
}

.highlight-badge {
	position: absolute;
	top: -2px;
	right: -2px;
	transform: translateX(calc(100% + 6px));
	display: flex;
	align-items: center;
	justify-content: center;
	width: 18px;
	height: 18px;
	border-radius: 6px;
	background: var(--content-link);
	color: var(--content-primary);
}

.highlight-badge svg {
	width: 14px;
	height: 14px;
}

/* When an edge is clamped to the viewport, decorations that hang off it
   move inside the box so they stay visible. */
.highlight.at-top .highlight-label {
	bottom: auto;
	top: 6px;
	left: 6px;
	transform: none;
}


.highlight.at-right .highlight-badge {
	right: 6px;
	transform: none;
}

.highlight.at-top .highlight-badge {
	top: 6px;
}

.highlight-label {
	position: absolute;
	left: -2px;
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


/* "Agent working" state: a faint tint, a comet of light circling the
   border, and an occasional soft diagonal glint. Everything animates on
   the compositor (transform only) and never obscures the element. */
.shimmer {
	position: fixed;
	z-index: 2147483645;
	pointer-events: none;
	overflow: hidden;
	border-radius: var(--radius-lg);
	background: hsl(213 94% 68% / 0.06);
}

/* A ring the same width as the selection outline: the element is masked
   to its padding box edge so only the border area shows, and the rotating
   conic gradient inside it reads as a beam travelling around a steady
   dim outline. */
.shimmer .beam {
	position: absolute;
	inset: 0;
	padding: var(--outline-width);
	border-radius: inherit;
	-webkit-mask:
		linear-gradient(#000 0 0) content-box,
		linear-gradient(#000 0 0);
	-webkit-mask-composite: xor;
	mask:
		linear-gradient(#000 0 0) content-box,
		linear-gradient(#000 0 0);
	mask-composite: exclude;
}

.shimmer .beam::before {
	content: "";
	position: absolute;
	left: 50%;
	top: 50%;
	width: var(--diagonal, 100%);
	height: var(--diagonal, 100%);
	margin: calc(var(--diagonal, 100%) / -2) 0 0 calc(var(--diagonal, 100%) / -2);
	border-radius: 50%;
	background: conic-gradient(
		from 0deg,
		hsl(213 94% 68% / 0.3) 0 55%,
		hsl(213 94% 68% / 0.5) 75%,
		hsl(213 94% 68%) 88%,
		hsl(0 0% 85%) 94%,
		hsl(213 94% 68% / 0.3) 100%
	);
	animation: coder-beam 3s linear infinite;
	will-change: transform;
}

/* A -32deg #D9D9D9 glint that crosses the box, rests off-screen, then
   returns. The band is a rotated strip with its gradient running across
   its own width, so its edges sit exactly on the transparent stops and
   there is no hard seam where the layer ends. It is sized from the box
   diagonal so it always spans the box, and slides along its own axis. */
.shimmer::after {
	content: "";
	position: absolute;
	left: 50%;
	top: 50%;
	width: calc(var(--diagonal, 100%) * 0.6);
	height: calc(var(--diagonal, 100%) * 2);
	margin: calc(var(--diagonal, 100%) * -1) 0 0
		calc(var(--diagonal, 100%) * -0.3);
	background: linear-gradient(
		90deg,
		hsl(0 0% 85% / 0) 0%,
		hsl(0 0% 85% / 0.16) 50%,
		hsl(0 0% 85% / 0) 100%
	);
	animation: coder-glint 3.6s cubic-bezier(0.45, 0, 0.2, 1) infinite;
	will-change: transform;
}

@media (prefers-reduced-motion: reduce) {
	.shimmer .beam::before,
	.shimmer::after {
		animation: none;
	}

	/* A steady ring stands in for the moving beam. */
	.shimmer .beam::before {
		background: hsl(213 94% 68% / 0.6);
	}
}

@keyframes coder-beam {
	to {
		transform: rotate(1turn);
	}
}

@keyframes coder-glint {
	0% {
		transform: rotate(-32deg) translateX(-200%);
	}
	55%,
	100% {
		transform: rotate(-32deg) translateX(200%);
	}
}

.popup {
	position: fixed;
	z-index: 2147483647;
	width: 24rem;
	max-width: calc(100vw - 16px);
	padding: 1.25rem;
	display: flex;
	flex-direction: column;
	gap: 1rem;
	background: var(--surface-primary);
	border: 1px solid var(--border);
	border-radius: var(--radius-lg);
	box-shadow: var(--shadow-lg);
}

.popup-title {
	font-size: 0.875rem;
	line-height: 1.25rem;
	font-weight: 500;
	color: var(--content-secondary);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.popup-target {
	font-family: var(--font-mono);
}

.popup textarea {
	font-family: var(--font-sans);
	font-size: 0.875rem;
	line-height: 1.5rem;
	width: 100%;
	min-height: 5.5rem;
	resize: vertical;
	padding: 0.5rem 0.75rem;
	border-radius: var(--radius-md);
	border: 1px solid var(--border);
	background: transparent;
	color: var(--content-primary);
}

.popup textarea::placeholder {
	color: var(--content-secondary);
}

.popup-actions {
	display: flex;
	justify-content: flex-end;
	gap: 0.5rem;
}

/* Button size=sm variants default and outline. */
.button {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	gap: 0.25rem;
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

/* Button variant=default disabled: surface-secondary and content-disabled. */
.button[disabled] {
	pointer-events: none;
	background: var(--surface-secondary);
	color: var(--content-disabled);
}

.button svg {
	width: 1rem;
	height: 1rem;
	flex: none;
}

.button.outline {
	background: transparent;
	border-color: var(--border);
	color: var(--content-primary);
	font-weight: 500;
}

.button.outline:hover {
	background: var(--surface-secondary);
}
`;

export const pickingCursorStyles =
	"*, *::before, *::after { cursor: crosshair !important; }";
