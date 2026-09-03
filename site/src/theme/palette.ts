/**
 * Minimal palette used by remaining non-Tailwind consumers (Monaco, charts,
 * confetti, color pickers). Prefer semantic CSS variables and roles when possible.
 */
export interface Palette {
	mode: "dark" | "light";
	primary: {
		main: string;
	};
	secondary: {
		main: string;
	};
	background: {
		default: string;
		paper: string;
	};
	text: {
		primary: string;
	};
}
