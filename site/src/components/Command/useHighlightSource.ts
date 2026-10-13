import { useState } from "react";

type HighlightSource = "keyboard" | "pointer";

// cmdk highlights a row on both arrow keys and pointer hover without
// recording which one did it. The last input on the page decides how a menu
// starts when it mounts or takes focus, so a menu opened or tabbed into from
// the keyboard shows the ring on its first highlighted row right away.
let lastPageInput: HighlightSource = "pointer";
if (typeof document !== "undefined") {
	document.addEventListener(
		"keydown",
		() => {
			lastPageInput = "keyboard";
		},
		true,
	);
	document.addEventListener(
		"pointerdown",
		() => {
			lastPageInput = "pointer";
		},
		true,
	);
}

/**
 * Tracks whether the keyboard or the pointer last moved the highlight inside a
 * cmdk root. Set `source` as the root's `data-highlight-source` and call the
 * marks from its focus, key down, and pointer move handlers. Items then show
 * the ring only for keyboard highlights via `keyboardHighlightRingClassName`.
 */
export const useHighlightSource = () => {
	const [source, setSource] = useState<HighlightSource>(() => lastPageInput);
	return {
		source,
		markFromPage: () => setSource(lastPageInput),
		markKeyboard: () => setSource("keyboard"),
		markPointer: () => setSource("pointer"),
	};
};

export const keyboardHighlightRingClassName =
	"in-data-[highlight-source=keyboard]:data-[selected=true]:ring-2 in-data-[highlight-source=keyboard]:data-[selected=true]:ring-inset in-data-[highlight-source=keyboard]:data-[selected=true]:ring-content-link";
