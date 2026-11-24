import { el } from "./dom";
import { elementLabel } from "./elementLabel";
import { sendIcon } from "./icons";

type CommentPopupOptions = {
	target: Element;
	onSubmit(comment: string): void;
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

	const send = el(doc, "button", "button", { type: "button", disabled: "" });
	send.innerHTML = sendIcon;
	send.append("Send");

	const submit = () => {
		const comment = textarea.value.trim();
		if (!comment) {
			textarea.focus();
			return;
		}
		options.onSubmit(comment);
	};
	send.addEventListener("click", submit);
	textarea.addEventListener("input", () => {
		send.disabled = textarea.value.trim() === "";
	});
	textarea.addEventListener("keydown", (event) => {
		if (event.key === "Enter" && !event.shiftKey) {
			event.preventDefault();
			submit();
		}
	});

	const actions = el(doc, "div", "popup-actions");
	actions.append(cancel, send);
	element.append(title, textarea, actions);

	return {
		element,
		focus: () => textarea.focus({ preventScroll: true }),
		reposition: () => {
			const target = options.target.getBoundingClientRect();
			const { width, height } = element.getBoundingClientRect();
			const left = Math.max(
				popupGap,
				Math.min(target.left, win.innerWidth - width - popupGap),
			);
			const below = target.bottom + popupGap;
			const top =
				below + height <= win.innerHeight - popupGap
					? below
					: Math.max(
							popupGap,
							Math.min(
								target.top - height - popupGap,
								win.innerHeight - height - popupGap,
							),
						);
			element.style.left = `${left}px`;
			element.style.top = `${top}px`;
		},
	};
}
