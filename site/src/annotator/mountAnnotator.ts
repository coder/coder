import { type CommentPopup, createCommentPopup } from "./commentPopup";
import { describeElement } from "./describeElement";
import { el, flagCutEdges, placeOver } from "./dom";
import { outlineInset, viewportBox } from "./geometry";
import { createHighlightLayer } from "./highlights";
import { checkIcon, pointerIcon } from "./icons";
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
};

export const annotatorHostId = "coder-annotator-host";

// The mount each document currently has, so a remount (Storybook, tests)
// tears down ours and only ours: the host id is namespaced but the page
// is not obliged to leave it alone.
const mounted = new WeakMap<Document, AnnotatorHandle>();

// Pointer events the page must not react to while picking. Their default
// actions are left alone so text selection still works and browsers do
// not suppress the click that follows a cancelled pointerdown.
const swallowedEvents = ["mousedown", "mouseup", "pointerdown", "pointerup"];

// Matches the `sent-flash` animation length in styles plus the chip's
// linger.
const sentFlashMs = 1600;

/**
 * Mounts the annotation overlay into the given document. Everything lives
 * inside a shadow root so the host page's styles cannot leak in and ours
 * cannot leak out. Returns a handle used by the postMessage bridge and by
 * Storybook.
 *
 * While picking, capturing listeners on the window run ahead of anything
 * the page registered on its document or elements: the pointer moves the
 * outline and a click opens the comment popup for the element under it
 * instead of reaching the page.
 */
export function mountAnnotator(
	options: MountAnnotatorOptions,
): AnnotatorHandle {
	const doc = options.document;
	const win = doc.defaultView;
	if (!win) {
		throw new Error("annotator requires a document attached to a window");
	}
	// Replace an earlier mount of ours, listeners included, so two overlays
	// never compete for the same clicks.
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

	const notify = () => {
		options.onStateChange?.({ picking });
	};

	// Origin and path only: query strings and fragments routinely carry
	// tokens, and the dashboard strips them again on receipt. Both fields
	// are host-controlled, so they are trimmed here rather than cloned into
	// the dashboard in full for its parser to cut down.
	const pageInfo = () => ({
		url: `${win.location.origin}${win.location.pathname}`.slice(
			0,
			maxFieldLength,
		),
		title: doc.title.slice(0, maxFieldLength),
		viewport: { width: win.innerWidth, height: win.innerHeight },
	});

	// Every saved comment is its own submission; there is no batching.
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
		const page = pageInfo();
		options.onSubmit({ page, annotations: [annotation] });
		flashSent(target);
		// Hold a quiet ring on the element until the dashboard reports the
		// agent working on it, so the send and the shimmer read as one
		// continuous state rather than two events with a gap between.
		highlights.markPending({
			id: annotation.id,
			selector: annotation.element.selector,
			url: page.url,
		});
	};

	// A one-shot pulse of the outline plus a "Sent" chip where the badge
	// sits, so the user sees the comment went without looking away.
	const flashSent = (target: Element) => {
		const flash = el(doc, "div", "sent-flash", { "aria-hidden": "true" });
		placeOver(
			flash,
			viewportBox(target.getBoundingClientRect(), win, outlineInset),
		);
		const chip = el(doc, "span", "sent-chip");
		chip.innerHTML = checkIcon;
		chip.append("Sent");
		flash.append(chip);
		shadow.append(flash);
		flagCutEdges(flash, win);
		win.setTimeout(() => flash.remove(), sentFlashMs);
	};

	// The outline stays on the element a comment is being written for and
	// otherwise follows the pointer.
	const layout = () => {
		outline.follow(picking ? (session?.target ?? hovered) : null);
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
		session?.popup.element.remove();
		session = null;
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
		session = { target, popup };
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

	// Escape closes an open comment first and leaves annotate mode
	// otherwise, wherever focus is. The dashboard mirrors the latter for
	// when focus is outside the preview.
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
