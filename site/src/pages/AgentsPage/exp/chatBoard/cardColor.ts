import { cva } from "class-variance-authority";

// The highlight tokens flip saturation with the color mode, so one set
// serves both. Decorative only, never status.
//
// The 3px edge is the 1px border recolored plus a 2px stripe drawn over the
// padding, so coloring a card does not move its content. The stripe sits
// above the header tint and lets clicks through to the color picker.
const stripe =
	"relative before:pointer-events-none before:absolute before:inset-y-0 before:left-0 before:z-[2] before:w-[2px]";

export const cardAccent = cva("", {
	variants: {
		color: {
			green: `${stripe} border-l-highlight-green before:bg-highlight-green`,
			orange: `${stripe} border-l-highlight-orange before:bg-highlight-orange`,
			sky: `${stripe} border-l-highlight-sky before:bg-highlight-sky`,
			red: `${stripe} border-l-highlight-red before:bg-highlight-red`,
			purple: `${stripe} border-l-highlight-purple before:bg-highlight-purple`,
			magenta: `${stripe} border-l-highlight-magenta before:bg-highlight-magenta`,
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
