export const annotatorStyles = /* css */ `
:host {
	all: initial;
	--bg: #18181b;
	--bg-raised: #27272a;
	--fg: #fafafa;
	--fg-muted: #a1a1aa;
	--border: #3f3f46;
	--accent: #8b5cf6;
	--accent-fg: #ffffff;
	--danger: #ef4444;
	--radius: 8px;
	--shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
	font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
	font-size: 13px;
	line-height: 1.4;
	color: var(--fg);
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
	outline: 2px solid var(--accent);
	outline-offset: 1px;
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
	background: var(--bg);
	border: 1px solid var(--border);
	border-radius: 999px;
	box-shadow: var(--shadow);
}

.toolbar-button {
	display: inline-flex;
	align-items: center;
	gap: 6px;
	height: 28px;
	padding: 0 10px;
	border-radius: 999px;
	color: var(--fg-muted);
	white-space: nowrap;
}

.toolbar-button:hover {
	background: var(--bg-raised);
	color: var(--fg);
}

.toolbar-button[aria-pressed="true"] {
	background: var(--accent);
	color: var(--accent-fg);
}

.toolbar-button[disabled] {
	opacity: 0.5;
	cursor: not-allowed;
}

.toolbar-button svg {
	width: 14px;
	height: 14px;
	flex: none;
}

.count {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	min-width: 18px;
	height: 18px;
	padding: 0 5px;
	border-radius: 999px;
	background: var(--bg-raised);
	color: var(--fg);
	font-size: 11px;
	font-weight: 600;
}

.toolbar-button[aria-pressed="true"] .count {
	background: rgba(255, 255, 255, 0.25);
}

.highlight {
	position: fixed;
	z-index: 2147483646;
	pointer-events: none;
	border: 2px solid var(--accent);
	background: rgba(139, 92, 246, 0.12);
	border-radius: 2px;
	display: none;
}

.highlight-label {
	position: absolute;
	left: -2px;
	bottom: 100%;
	transform: translateY(-4px);
	padding: 2px 6px;
	border-radius: 4px;
	background: var(--accent);
	color: var(--accent-fg);
	font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	font-size: 11px;
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
	width: 22px;
	height: 22px;
	border-radius: 999px;
	background: var(--accent);
	color: var(--accent-fg);
	font-size: 11px;
	font-weight: 700;
	box-shadow: var(--shadow);
	transform: translate(-50%, -50%);
}

.pin:hover {
	filter: brightness(1.15);
}

.popup {
	position: fixed;
	z-index: 2147483647;
	width: 320px;
	padding: 10px;
	display: flex;
	flex-direction: column;
	gap: 8px;
	background: var(--bg);
	border: 1px solid var(--border);
	border-radius: var(--radius);
	box-shadow: var(--shadow);
}

.popup-target {
	font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
	font-size: 11px;
	color: var(--fg-muted);
	white-space: nowrap;
	overflow: hidden;
	text-overflow: ellipsis;
}

.popup textarea {
	font: inherit;
	width: 100%;
	min-height: 64px;
	resize: vertical;
	padding: 6px 8px;
	border-radius: 6px;
	border: 1px solid var(--border);
	background: var(--bg-raised);
	color: var(--fg);
}

.popup textarea::placeholder {
	color: var(--fg-muted);
}

.popup-actions {
	display: flex;
	align-items: center;
	gap: 6px;
}

.popup-actions .spacer {
	flex: 1;
}

.button {
	height: 26px;
	padding: 0 10px;
	border-radius: 6px;
	background: var(--bg-raised);
	color: var(--fg);
}

.button:hover {
	filter: brightness(1.15);
}

.button.primary {
	background: var(--accent);
	color: var(--accent-fg);
}

.button.danger {
	color: var(--danger);
}

.hint {
	color: var(--fg-muted);
	font-size: 11px;
}
`;

export const pickingCursorStyles =
	"*, *::before, *::after { cursor: crosshair !important; }";
