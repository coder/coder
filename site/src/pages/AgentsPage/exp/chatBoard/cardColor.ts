import { cva } from "class-variance-authority";

// The highlight tokens flip saturation with the color mode, so one set
// serves both. Decorative only, never status.
export const cardAccent = cva("", {
	variants: {
		color: {
			green: "border-l-[3px] border-l-highlight-green",
			orange: "border-l-[3px] border-l-highlight-orange",
			sky: "border-l-[3px] border-l-highlight-sky",
			red: "border-l-[3px] border-l-highlight-red",
			purple: "border-l-[3px] border-l-highlight-purple",
			magenta: "border-l-[3px] border-l-highlight-magenta",
		},
	},
});

export const cardTint = cva("", {
	variants: {
		color: {
			green: "bg-highlight-green/15",
			orange: "bg-highlight-orange/15",
			sky: "bg-highlight-sky/15",
			red: "bg-highlight-red/15",
			purple: "bg-highlight-purple/15",
			magenta: "bg-highlight-magenta/15",
		},
	},
});

/** Solid fill in a card's color: the palette swatches and a window's title dot. */
export const cardSwatch = cva("", {
	variants: {
		color: {
			green: "bg-highlight-green",
			orange: "bg-highlight-orange",
			sky: "bg-highlight-sky",
			red: "bg-highlight-red",
			purple: "bg-highlight-purple",
			magenta: "bg-highlight-magenta",
		},
	},
});
