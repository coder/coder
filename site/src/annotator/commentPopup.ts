import { el } from "./dom";
import { elementLabel } from "./elementLabel";
import { plusIcon, sendIcon } from "./icons";

type CommentPopupOptions = {
	target: Element;
	// Comments already held back, which the next plain Send includes.
	heldCount: number;
	// `hold` is Shift+Send: keep the comment and pick more first.
	onSubmit(comment: string, hold: boolean): void;
	onCancel(): void;
};

export type CommentPopup = {
	element: HTMLDivElement;
	focus(): void;
};

const popupWidth = 384;
// Tall enough for the title, three rows and the actions; the popup flips
// above the target when this would not fit below it.
const popupHeight = 220;
const popupGap = 8;

/**
 * Where to put the popup so it sits just below the target, or above it
 * when there is no room, and stays inside the viewport horizontally.
 */
export function popupPosition(
	target: DOMRect,
	viewport: Pick<Window, "innerWidth" | "innerHeight">,
): { left: number; top: number } {
	const left = Math.max(
		popupGap,
		Math.min(target.left, viewport.innerWidth - popupWidth - popupGap),
	);
	const below = target.bottom + popupGap;
	const fitsBelow = below + popupHeight <= viewport.innerHeight;
	const top = fitsBelow
		? below
		: Math.max(popupGap, target.top - popupHeight - popupGap);
	return { left, top };
}

/**
 * The comment box for a picked element: a title naming the element, a
 * textarea, Cancel and Send. Enter sends, Shift+Enter makes a new line,
 * Shift+Send holds the comment for a later Send and an empty comment
 * cannot be sent. Positioned for the caller to append into the overlay.
 */
export function createCommentPopup(
	win: Window,
	options: CommentPopupOptions,
): CommentPopup {
	const doc = win.document;
	const element = el(doc, "div", "popup", {
		role: "dialog",
		"aria-label": "Annotate element",
	});

	const title = el(doc, "div", "popup-title");
	title.textContent = "Comment";
	const targetName = el(doc, "span", "popup-target");
	targetName.textContent = `(${elementLabel(options.target)})`;
	title.append(" ", targetName);

	const textarea = el(doc, "textarea", undefined, {
		placeholder: "What should change here?",
		"aria-label": "Annotation comment",
		rows: "3",
	});

	const cancel = el(doc, "button", "button outline", { type: "button" });
	cancel.textContent = "Cancel";
	cancel.addEventListener("click", options.onCancel);

	const send = el(doc, "button", "button", {
		type: "button",
		disabled: "",
		"data-tip": "Shift+click to hold this comment and pick more",
	});
	const setSendLabel = (hold: boolean) => {
		send.innerHTML = hold ? plusIcon : sendIcon;
		const count = options.heldCount;
		send.append(hold ? "Add" : count > 0 ? `Send ${count + 1}` : "Send");
	};
	setSendLabel(false);

	const submit = (hold: boolean) => {
		const comment = textarea.value.trim();
		if (!comment) {
			textarea.focus();
			return;
		}
		options.onSubmit(comment, hold);
	};
	send.addEventListener("click", (event) => submit(event.shiftKey));
	textarea.addEventListener("input", () => {
		send.disabled = textarea.value.trim() === "";
	});
	textarea.addEventListener("keydown", (event) => {
		if (event.key === "Enter" && !event.shiftKey) {
			event.preventDefault();
			submit(false);
		}
	});
	// Holding Shift previews what the button will do.
	const onShift = (event: KeyboardEvent) => setSendLabel(event.shiftKey);
	element.addEventListener("keydown", onShift);
	element.addEventListener("keyup", onShift);

	const actions = el(doc, "div", "popup-actions");
	actions.append(cancel, send);
	element.append(title, textarea, actions);

	const { left, top } = popupPosition(
		options.target.getBoundingClientRect(),
		win,
	);
	element.style.left = `${left}px`;
	element.style.top = `${top}px`;

	return { element, focus: () => textarea.focus() };
}
