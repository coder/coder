import { el } from "./dom";
import { closeIcon } from "./icons";

type HintPill = {
	element: HTMLElement;
	// Asks for the pill to show while picking, until it is dismissed.
	want(): void;
	// Matches the pill's visibility to picking mode.
	sync(picking: boolean): void;
	// Hides the pill for good and reports that, once.
	dismiss(): void;
};

/**
 * First-run hint: a pill at the top of the page while picking, until the
 * user closes it or sends a first comment.
 */
export function createHintPill(
	doc: Document,
	onDismissed: () => void,
): HintPill {
	const element = el(doc, "div", "hint-pill", { role: "status" });
	const text = el(doc, "span");
	text.textContent = "Select an item to request updates. Press esc to exit.";
	const close = el(doc, "button", "hint-close", {
		type: "button",
		"aria-label": "Dismiss hint",
	});
	close.innerHTML = closeIcon;
	element.append(text, close);
	element.style.display = "none";

	let wanted = false;
	const show = (visible: boolean) => {
		element.style.display = visible ? "flex" : "none";
	};
	const dismiss = () => {
		if (!wanted) {
			return;
		}
		wanted = false;
		show(false);
		onDismissed();
	};
	close.addEventListener("click", dismiss);

	return {
		element,
		want: () => {
			wanted = true;
		},
		sync: (picking) => show(picking && wanted),
		dismiss,
	};
}
