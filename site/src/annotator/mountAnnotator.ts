import { type CommentPopup, createCommentPopup } from "./commentPopup";
import { describeElement } from "./describeElement";
import { el } from "./dom";
import { createHighlightLayer } from "./highlights";
import { pointerIcon } from "./icons";
import pickingCursorStyles from "./pickingCursor.css?inline";
import { createPickOutline } from "./pickOutline";
import { isOwnEvent, pickTarget } from "./pickTarget";
import {
	type Annotation,
	type AnnotationSubmission,
	annotationIdAttribute,
	type HighlightItem,
	maxFieldLength,
} from "./protocol";
import { randomId } from "./randomId";
import annotatorStyles from "./styles.css?inline";

type AnnotatorState = {
	picking: boolean;
};

type AnnotatorHandle = {
	setPicking(picking: boolean): void;
	setHighlights(items: HighlightItem[]): void;
	getState(): AnnotatorState;
	destroy(): void;
};

type MountAnnotatorOptions = {
	document: Document;
	onSubmit(submission: AnnotationSubmission): void;
	onStateChange?(state: AnnotatorState): void;
};

// The comment being written and which element it is about. The outline
// stays on that element while the popup is open.
type CommentSession = {
	target: Element;
	popup: CommentPopup;
	previousFocus: Element | null;
};

/** Identifies the overlay host, which may coexist with a page-owned matching ID. */
export const annotatorHostId = "coder-annotator-host";

// The mount each document currently has, so a remount (Storybook, tests)
// tears down ours and only ours: the host id is namespaced but the page
// is not obliged to leave it alone.
const mounted = new WeakMap<Document, AnnotatorHandle>();

// Pointer events the page must not react to while picking. Their default
// actions are left alone so text selection still works and browsers do
// not suppress the click that follows a cancelled pointerdown.
const swallowedEvents = ["mousedown", "mouseup", "pointerdown", "pointerup"];

function focusedElement(doc: Document): Element | null {
	let active = doc.activeElement;
	while (active?.shadowRoot?.activeElement) {
		active = active.shadowRoot.activeElement;
	}
	return active;
}

/**
 * Isolates the overlay in a shadow root and intercepts page clicks in the
 * capture phase while picking. Remounting replaces only our previous overlay.
 */
export function mountAnnotator(
	options: MountAnnotatorOptions,
): AnnotatorHandle {
	const doc = options.document;
	const win = doc.defaultView;
	if (!win) {
		throw new Error("annotator requires a document attached to a window");
	}
	mounted.get(doc)?.destroy();

	const host = el(doc, "div");
	host.id = annotatorHostId;
	const shadow = host.attachShadow({ mode: "open" });
	const style = el(doc, "style");
	style.textContent = annotatorStyles;

	// Annotate mode is started from the dashboard, the only side that can
	// vouch for what the overlay sends. The toolbar offers to stop it and is
	// hidden while idle rather than showing a control that cannot start.
	const toolbar = el(doc, "div", "toolbar", {
		role: "toolbar",
		"aria-label": "UI annotations",
	});
	const stopButton = el(doc, "button", "icon-button", {
		type: "button",
		"aria-pressed": "true",
		"aria-label": "Stop annotating",
		"data-tip": "Stop annotating",
	});
	stopButton.innerHTML = pointerIcon;
	toolbar.append(stopButton);
	toolbar.style.display = "none";

	const outline = createPickOutline(win);
	const highlightsContainer = el(doc, "div", "highlights", {
		"aria-hidden": "true",
	});
	shadow.append(style, toolbar, outline.element, highlightsContainer);
	const highlights = createHighlightLayer(doc, win, highlightsContainer);
	doc.body.append(host);

	// Added to the page's own head while picking, since the cursor rule has
	// to apply to page elements rather than to anything in the shadow root.
	const cursorStyle = el(doc, "style");
	cursorStyle.textContent = pickingCursorStyles;

	let picking = false;
	let hovered: Element | null = null;
	let session: CommentSession | null = null;
	let layoutFrame = 0;
	let focusBeforePicking: Element | null = null;

	const restoreFocus = (element: Element | null): boolean => {
		if (
			!element?.isConnected ||
			element === doc.body ||
			!("focus" in element) ||
			typeof element.focus !== "function"
		) {
			return false;
		}
		element.focus({ preventScroll: true });
		return focusedElement(doc) === element;
	};

	const notify = () => {
		options.onStateChange?.({ picking });
	};

	// Query strings and fragments routinely carry tokens. Bound the remaining
	// page-controlled fields before cloning them into the dashboard.
	const pageInfo = () => ({
		url: `${win.location.origin}${win.location.pathname}`.slice(
			0,
			maxFieldLength,
		),
		title: doc.title.slice(0, maxFieldLength),
		viewport: { width: win.innerWidth, height: win.innerHeight },
	});

	const submitComment = (
		target: Element,
		comment: string,
		selectedText: string | undefined,
	) => {
		const annotation: Annotation = {
			id: randomId(),
			comment,
			selectedText,
			element: describeElement(target),
		};
		// Stamped after describing so the marker never leaks into the
		// captured selector or opening tag.
		target.setAttribute(annotationIdAttribute, annotation.id);
		options.onSubmit({ page: pageInfo(), annotations: [annotation] });
	};

	// The outline stays on the element a comment is being written for and
	// otherwise follows the pointer.
	const layout = () => {
		outline.follow(picking ? (session?.target ?? hovered) : null);
		session?.popup.reposition();
	};

	const scheduleLayout = () => {
		if (layoutFrame !== 0) {
			return;
		}
		layoutFrame = win.requestAnimationFrame(() => {
			layoutFrame = 0;
			layout();
		});
	};

	const closePopup = () => {
		if (!session) {
			return;
		}
		const { popup, previousFocus } = session;
		const hadFocus = popup.element.contains(shadow.activeElement);
		popup.element.remove();
		session = null;
		if (hadFocus && !restoreFocus(previousFocus) && picking) {
			stopButton.focus({ preventScroll: true });
		}
		scheduleLayout();
	};

	const openPopup = (target: Element, selectedText: string | undefined) => {
		closePopup();
		const popup = createCommentPopup(win, {
			target,
			onCancel: closePopup,
			onSubmit: (comment) => {
				submitComment(target, comment, selectedText);
				closePopup();
			},
		});
		session = { target, popup, previousFocus: focusedElement(doc) };
		shadow.append(popup.element);
		layout();
		popup.focus();
	};

	const onPointerMove = (event: MouseEvent) => {
		if (session) {
			return;
		}
		const next = pickTarget(host, event);
		if (next !== hovered) {
			hovered = next;
			scheduleLayout();
		}
	};

	const swallow = (event: Event) => {
		if (!isOwnEvent(host, event)) {
			event.stopImmediatePropagation();
		}
	};

	// Clicking away from an open popup dismisses it; otherwise a click picks
	// the element under the pointer, with any text selected at the time.
	const onClick = (event: MouseEvent) => {
		if (isOwnEvent(host, event)) {
			return;
		}
		event.stopImmediatePropagation();
		if (session) {
			event.preventDefault();
			closePopup();
			return;
		}
		const target = pickTarget(host, event);
		if (!target) {
			return;
		}
		event.preventDefault();
		const selection = win.getSelection()?.toString().trim();
		openPopup(target, selection ? selection.slice(0, 500) : undefined);
	};

	// Escape dismisses the draft before leaving annotate mode, wherever focus is.
	const onKeyDown = (event: KeyboardEvent) => {
		if (event.key !== "Escape") {
			return;
		}
		event.preventDefault();
		event.stopImmediatePropagation();
		if (session) {
			closePopup();
		} else {
			setPicking(false);
		}
	};

	const setPicking = (next: boolean) => {
		if (next === picking) {
			return;
		}
		const restorePageFocus = !next && doc.activeElement === host;
		if (next) {
			focusBeforePicking = focusedElement(doc);
		}
		picking = next;
		toolbar.style.display = next ? "flex" : "none";
		if (next) {
			doc.head.append(cursorStyle);
			win.addEventListener("mousemove", onPointerMove, true);
			win.addEventListener("click", onClick, true);
			win.addEventListener("keydown", onKeyDown, true);
			for (const type of swallowedEvents) {
				win.addEventListener(type, swallow, true);
			}
		} else {
			cursorStyle.remove();
			win.removeEventListener("mousemove", onPointerMove, true);
			win.removeEventListener("click", onClick, true);
			win.removeEventListener("keydown", onKeyDown, true);
			for (const type of swallowedEvents) {
				win.removeEventListener(type, swallow, true);
			}
			hovered = null;
			closePopup();
			if (restorePageFocus) {
				restoreFocus(focusBeforePicking);
			}
		}
		scheduleLayout();
		notify();
	};

	stopButton.addEventListener("click", () => setPicking(false));
	win.addEventListener("scroll", scheduleLayout, true);
	win.addEventListener("resize", scheduleLayout);
	notify();

	const handle: AnnotatorHandle = {
		setPicking,
		setHighlights: highlights.set,
		getState: () => ({ picking }),
		destroy: () => {
			setPicking(false);
			highlights.destroy();
			win.removeEventListener("scroll", scheduleLayout, true);
			win.removeEventListener("resize", scheduleLayout);
			if (layoutFrame !== 0) {
				win.cancelAnimationFrame(layoutFrame);
			}
			host.remove();
			if (mounted.get(doc) === handle) {
				mounted.delete(doc);
			}
		},
	};
	mounted.set(doc, handle);
	return handle;
}
