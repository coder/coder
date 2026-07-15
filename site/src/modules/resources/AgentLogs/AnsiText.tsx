import {
	type CSSProperties,
	type FC,
	type ReactNode,
	useEffect,
	useMemo,
	useSyncExternalStore,
} from "react";

// The libghostty-vt grid this module parses into. Log lines soft-wrap across
// rows, so a single line can hold up to COLS * ROWS characters. Anything
// longer scrolls off the top and only the tail is kept, matching how a real
// terminal treats overlong output.
const COLS = 1024;
const ROWS = 64;

// Cell attribute bits from ghostty_cell_t (part of the libghostty-vt C ABI,
// mirrored from ghostty-web's CellFlags so the library can stay
// dynamically imported).
const BOLD = 1;
const ITALIC = 2;
const UNDERLINE = 4;
const STRIKETHROUGH = 8;
const INVERSE = 16;
const INVISIBLE = 32;
const FAINT = 128;

interface Cell {
	codepoint: number;
	fg_r: number;
	fg_g: number;
	fg_b: number;
	bg_r: number;
	bg_g: number;
	bg_b: number;
	flags: number;
	width: number;
}

interface Terminal {
	write(data: string): void;
	getLine(y: number): Cell[] | null;
	isRowWrapped(row: number): boolean;
	getGraphemeString(row: number, col: number): string;
}

let terminal: Terminal | undefined;
let defaultFg = "";
let defaultBg = "";
let loading: Promise<void> | undefined;
const readyListeners = new Set<() => void>();

// Loads the libghostty-vt WASM parser and creates the shared headless
// terminal. The library is imported dynamically so its embedded WASM blob
// stays out of the main bundle until agent logs are actually rendered.
export function loadGhostty(): Promise<void> {
	loading ??= import("ghostty-web").then(({ Ghostty }) =>
		Ghostty.load().then((ghostty) => {
			const term = ghostty.createTerminal(COLS, ROWS, { scrollbackLimit: 0 });
			// Probe the resolved default colors so unstyled cells can inherit
			// their color from the surrounding log line instead of hardcoding
			// the terminal theme's defaults.
			term.write("x");
			const probe = term.getLine(0)?.[0];
			if (probe) {
				defaultFg = rgb(probe.fg_r, probe.fg_g, probe.fg_b);
				defaultBg = rgb(probe.bg_r, probe.bg_g, probe.bg_b);
			}
			term.write("\u001bc");
			terminal = term;
			for (const listener of readyListeners) {
				listener();
			}
		}),
	);
	return loading;
}

const subscribe = (listener: () => void) => {
	readyListeners.add(listener);
	return () => readyListeners.delete(listener);
};
const isReady = () => terminal !== undefined;

const rgb = (r: number, g: number, b: number) => `rgb(${r},${g},${b})`;

interface Run {
	key: string;
	style: CSSProperties | undefined;
	text: string;
}

const cellStyle = (cell: Cell): [string, CSSProperties | undefined] => {
	let fg = rgb(cell.fg_r, cell.fg_g, cell.fg_b);
	let bg = rgb(cell.bg_r, cell.bg_g, cell.bg_b);
	if (cell.flags & INVERSE) {
		[fg, bg] = [bg, fg];
	}

	const style: CSSProperties = {};
	if (fg !== defaultFg) {
		style.color = fg;
	}
	if (bg !== defaultBg) {
		style.backgroundColor = bg;
	}
	if (cell.flags & BOLD) {
		style.fontWeight = 600;
	}
	if (cell.flags & FAINT) {
		style.opacity = 0.7;
	}
	if (cell.flags & ITALIC) {
		style.fontStyle = "italic";
	}
	const decorations = [
		cell.flags & UNDERLINE ? "underline" : "",
		cell.flags & STRIKETHROUGH ? "line-through" : "",
	]
		.filter(Boolean)
		.join(" ");
	if (decorations) {
		style.textDecoration = decorations;
	}
	if (cell.flags & INVISIBLE) {
		style.visibility = "hidden";
	}

	const entries = Object.entries(style);
	if (entries.length === 0) {
		return ["", undefined];
	}
	return [entries.flat().join(";"), style];
};

// Converts a log line to styled React nodes by writing it through the shared
// terminal and reading the resulting cells back. Runs of identically styled
// cells are coalesced into single spans. Returns the raw text when the parser
// has not finished loading yet.
const ansiToReact = (text: string): ReactNode => {
	const term = terminal;
	if (!term) {
		return text;
	}

	// RIS: full reset (cursor, SGR state, screen) between lines.
	term.write("\u001bc");
	term.write(text);

	const runs: Run[] = [];
	for (let y = 0; y < ROWS; y++) {
		if (y > 0 && !term.isRowWrapped(y)) {
			break;
		}
		const cells = term.getLine(y);
		if (!cells) {
			break;
		}
		for (let x = 0; x < cells.length; x++) {
			const cell = cells[x];
			// Wide characters occupy two cells; the second is a spacer.
			if (cell.width === 0) {
				continue;
			}
			const [key, style] = cellStyle(cell);
			const ch =
				cell.codepoint === 0
					? " "
					: cell.codepoint < 0x80
						? String.fromCodePoint(cell.codepoint)
						: // Complex graphemes (combining marks, emoji ZWJ sequences)
							// span multiple codepoints; ask the terminal for the full
							// cluster rather than assuming one codepoint per cell.
							term.getGraphemeString(y, x);
			const run = runs.at(-1);
			if (run && run.key === key) {
				run.text += ch;
			} else {
				runs.push({ key, style, text: ch });
			}
		}
	}

	// Trim trailing cells that never received visible content: unstyled
	// whitespace at the end of the grid row.
	while (runs.length > 0) {
		const last = runs[runs.length - 1];
		if (last.style === undefined) {
			last.text = last.text.replace(/ +$/, "");
		}
		if (last.text.length === 0) {
			runs.pop();
		} else {
			break;
		}
	}

	return runs.map((run, i) =>
		run.style === undefined ? (
			run.text
		) : (
			<span key={i} style={run.style}>
				{run.text}
			</span>
		),
	);
};

interface AnsiTextProps {
	text: string;
}

// Renders ANSI-formatted text using Ghostty's VT parser (libghostty-vt via
// ghostty-web). Falls back to the raw text while the WASM parser loads.
export const AnsiText: FC<AnsiTextProps> = ({ text }) => {
	const ready = useSyncExternalStore(subscribe, isReady, isReady);
	useEffect(() => {
		if (!ready) {
			void loadGhostty();
		}
	}, [ready]);
	const children = useMemo(
		() => (ready ? ansiToReact(text) : text),
		[ready, text],
	);
	return <span>{children}</span>;
};
