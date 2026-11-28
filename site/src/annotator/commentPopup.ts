import { el } from "./dom";
import { elementLabel } from "./elementLabel";
import { plusIcon, sendIcon } from "./icons";
import { maxAnnotations } from "./protocol";

type CommentPopupOptions = {
	target: Element;
	toolbar: HTMLElement;
	// Comments already held back, which the next plain Send includes.
	heldCount: number;
	// `hold` is Shift+Send. Returns false when the batch cannot hold more.
	onSubmit(comment: string, hold: boolean): boolean;
	onCancel(): void;
};

/** Controls a comment dialog mounted in the overlay's shadow root. */
export type CommentPopup = {
	element: HTMLDivElement;
	focus(): void;
	reposition(): void;
};

const popupGap = 8;

/**
 * Creates a non-modal comment dialog. Append it before repositioning so
 * placement uses its rendered size rather than an assumed viewport or font.
 */
export function createCommentPopup(
	win: Window,
	options: CommentPopupOptions,
): CommentPopup {
	const doc = win.document;
	const element = el(doc, "div", "popup", {
		role: "dialog",
		"aria-labelledby": "comment-title",
	});

	const title = el(doc, "div", "popup-title", { id: "comment-title" });
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

	// sentence-case-expect: Shift is the keyboard key name.
	const limitMessage = `Up to ${maxAnnotations} comments per send. Send without Shift to submit this batch.`;
	const limit = el(doc, "div", "popup-limit", {
		id: "comment-limit",
		role: "status",
	});
	const atLimit = options.heldCount >= maxAnnotations - 1;
	if (atLimit) {
		limit.textContent = limitMessage;
	}
	const send = el(doc, "button", "button", {
		type: "button",
		disabled: "",
		"aria-describedby": "comment-limit",
	});
	if (!atLimit) {
		send.dataset.tip = "Shift+click to hold this comment and pick more";
	}
	const setSendLabel = (shift: boolean) => {
		const hold = shift && !atLimit;
		send.innerHTML = hold ? plusIcon : sendIcon;
		if (shift) {
			// sentence-case-expect: Shift is the keyboard key name.
			send.append(atLimit ? "Release Shift" : "Add");
			return;
		}
		const count = options.heldCount;
		send.append(count > 0 ? `Send ${count + 1}` : "Send");
	};
	setSendLabel(false);

	const submit = (hold: boolean) => {
		const comment = textarea.value.trim();
		if (!comment) {
			textarea.focus();
			return;
		}
		if (!options.onSubmit(comment, hold)) {
			limit.textContent = limitMessage;
			textarea.focus({ preventScroll: true });
		}
	};
	send.addEventListener("pointerenter", (event) =>
		setSendLabel(event.shiftKey),
	);
	send.addEventListener("click", (event) => {
		setSendLabel(event.shiftKey);
		submit(event.shiftKey);
	});
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
	element.append(title, textarea, limit, actions);

	return {
		element,
		focus: () => textarea.focus({ preventScroll: true }),
		reposition: () => {
			const target = options.target.getBoundingClientRect();
			const { width } = element.getBoundingClientRect();
			const left = Math.max(
				popupGap,
				Math.min(target.left, win.innerWidth - width - popupGap),
			);
			const toolbar = options.toolbar.getBoundingClientRect();
			const bottom =
				left < toolbar.right + popupGap &&
				left + width > toolbar.left - popupGap
					? Math.min(win.innerHeight, toolbar.top) - popupGap
					: win.innerHeight - popupGap;
			// Keep the Stop control reachable even when the batch helper wraps.
			element.style.maxHeight = `${Math.max(0, bottom - popupGap)}px`;
			const { height } = element.getBoundingClientRect();
			const below = target.bottom + popupGap;
			const top =
				below + height <= bottom
					? below
					: Math.max(
							popupGap,
							Math.min(target.top - height - popupGap, bottom - height),
						);
			element.style.left = `${left}px`;
			element.style.top = `${top}px`;
		},
	};
}
