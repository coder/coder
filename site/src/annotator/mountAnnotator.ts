import { describeElementWithSource } from "./describeElement";
import { outlineInset, viewportBox } from "./geometry";
import { createHighlightLayer, statusChip } from "./highlights";
import { carryMarkerAcrossNavigation } from "./navigation";
import pickingCursorStyles from "./pickingCursor.css?inline";
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
	// `hint` shows the first-run hint alongside picking mode.
	setPicking(picking: boolean, hint?: boolean): void;
	setHighlights(items: HighlightItem[]): void;
	// Acknowledges the listed annotations as changed by the agent and clears
	// the rest.
	resolveHighlights(ids: string[]): void;
	getState(): AnnotatorState;
	destroy(): void;
};

type MountAnnotatorOptions = {
	document: Document;
	onSubmit(submission: AnnotationSubmission): void;
	onStateChange?(state: AnnotatorState): void;
	// The user closed the first-run hint or sent their first comment.
	onHintDismissed?(): void;
};

type PopupSession = {
	target: Element;
	selectedText?: string;
};

export const annotatorHostId = "coder-annotator-host";

// The host element each document currently has mounted, so a remount
// (Storybook, tests) replaces ours without touching a page element that
// happens to share the id.
const mountedHosts = new WeakMap<Document, HTMLElement>();

// Matches the `sent-flash` animation length in styles plus the chip's
// linger.
const sentFlashMs = 1600;

const pointerIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 4.1 12 6"/><path d="m5.1 8-2.9-.8"/><path d="m6 12-1.9 2"/><path d="M7.2 2.2 8 5.1"/><path d="M9.037 9.69a.498.498 0 0 1 .653-.653l11 4.5a.5.5 0 0 1-.074.949l-4.349 1.041a1 1 0 0 0-.74.739l-1.04 4.35a.5.5 0 0 1-.95.074z"/></svg>';

const sendIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14.536 21.686a.5.5 0 0 0 .937-.024l6.5-19a.496.496 0 0 0-.635-.635l-19 6.5a.5.5 0 0 0-.024.937l7.93 3.18a2 2 0 0 1 1.112 1.11z"/><path d="m21.854 2.147-10.94 10.939"/></svg>';

const closeIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>';

const plusIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 12h14"/><path d="M12 5v14"/></svg>';

const sparklesIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594z"/><path d="M20 2v4"/><path d="M22 4h-4"/><circle cx="4" cy="20" r="2"/></svg>';

function el<K extends keyof HTMLElementTagNameMap>(
	doc: Document,
	tag: K,
	className?: string,
	attributes: Record<string, string> = {},
): HTMLElementTagNameMap[K] {
	const node = doc.createElement(tag);
	if (className) {
		node.className = className;
	}
	for (const [name, value] of Object.entries(attributes)) {
		node.setAttribute(name, value);
	}
	return node;
}

function shortLabel(target: Element): string {
	const tag = target.tagName.toLowerCase();
	if (target.id) {
		return `${tag}#${target.id}`;
	}
	const testId = target.getAttribute("data-testid");
	if (testId) {
		return `${tag}[data-testid=${testId}]`;
	}
	const firstClass = Array.from(target.classList).find(
		(name) => !name.includes(":") && !name.includes("["),
	);
	return firstClass ? `${tag}.${firstClass}` : tag;
}

/**
 * Mounts the annotation overlay into the given document. Everything lives
 * inside a shadow root so the host page's styles cannot leak in and ours
 * cannot leak out. Returns a handle used by the postMessage bridge and by
 * Storybook.
 */
export function mountAnnotator(
	options: MountAnnotatorOptions,
): AnnotatorHandle {
	const doc = options.document;
	const win = doc.defaultView;
	if (!win) {
		throw new Error("annotator requires a document attached to a window");
	}
	// Replace an earlier mount of ours, and only ours: the id is namespaced
	// but the page is not obliged to leave it alone.
	mountedHosts.get(doc)?.remove();

	const host = el(doc, "div");
	host.id = annotatorHostId;
	mountedHosts.set(doc, host);
	const shadow = host.attachShadow({ mode: "open" });
	const style = el(doc, "style");
	style.textContent = annotatorStyles;
	shadow.append(style);

	const toolbar = el(doc, "div", "toolbar", {
		role: "toolbar",
		"aria-label": "UI annotations",
	});
	// Annotate mode is started from the dashboard, the only side that can
	// vouch for what the overlay sends. The toolbar offers to stop it, in
	// the style of the dashboard's subtle icon Button and Tooltip, and is
	// hidden while idle rather than showing a control that cannot start.
	const pickButton = el(doc, "button", "icon-button", {
		type: "button",
		"aria-pressed": "true",
		"aria-label": "Stop annotating",
		"data-tip": "Stop annotating",
	});
	pickButton.innerHTML = pointerIcon;
	const heldBadge = el(doc, "button", "held-badge", {
		type: "button",
		"aria-label": "Discard held comments",
		"data-tip": "Comments waiting to be sent. Click to discard.",
	});
	heldBadge.style.display = "none";
	// A sibling rather than a child: buttons cannot nest.
	const pickWrap = el(doc, "span", "pick-wrap");
	pickWrap.append(pickButton, heldBadge);
	toolbar.append(pickWrap);
	toolbar.style.display = "none";

	// First-run hint: a pill at the top of the page while picking, until
	// the user closes it or sends a first comment.
	const hint = el(doc, "div", "hint-pill", { role: "status" });
	const hintText = el(doc, "span");
	hintText.textContent =
		"Select an item to request updates. Press esc to exit.";
	const hintClose = el(doc, "button", "hint-close", {
		type: "button",
		"aria-label": "Dismiss hint",
	});
	hintClose.innerHTML = closeIcon;
	hint.append(hintText, hintClose);
	hint.style.display = "none";
	let hintWanted = false;
	const showHint = (visible: boolean) => {
		hint.style.display = visible ? "flex" : "none";
	};
	const dismissHint = () => {
		if (!hintWanted) {
			return;
		}
		hintWanted = false;
		showHint(false);
		options.onHintDismissed?.();
	};
	hintClose.addEventListener("click", dismissHint);

	const highlight = el(doc, "div", "highlight", { "aria-hidden": "true" });
	const highlightLabel = el(doc, "span", "highlight-label");
	const highlightBadge = el(doc, "span", "highlight-badge");
	highlightBadge.innerHTML = sparklesIcon;
	highlight.append(highlightLabel, highlightBadge);

	const highlightsContainer = el(doc, "div", "highlights", {
		"aria-hidden": "true",
	});
	shadow.append(toolbar, hint, highlight, highlightsContainer);
	const highlights = createHighlightLayer(doc, win, highlightsContainer);
	doc.body.append(host);

	const cursorStyle = el(doc, "style");
	cursorStyle.textContent = pickingCursorStyles;

	let picking = false;
	// Comments held back with Shift+Send, sent together with the next
	// plain Send as one submission. Their elements keep a dashed outline.
	const held: { annotation: Annotation; target: Element; node: HTMLElement }[] =
		[];
	let hovered: Element | null = null;
	let popup: HTMLDivElement | null = null;
	// Element the open popup is about; outlined while the popup is open so
	// it stays clear what the comment applies to.
	let popupTarget: Element | null = null;
	let frame = 0;

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
	const createAnnotation = async (
		session: PopupSession,
		comment: string,
	): Promise<Annotation> => {
		const annotation: Annotation = {
			id: randomId(),
			comment,
			selectedText: session.selectedText,
			element: await describeElementWithSource(session.target),
		};
		// Stamped after describing so the marker never leaks into the
		// captured selector or opening tag.
		session.target.setAttribute(annotationIdAttribute, annotation.id);
		return annotation;
	};

	const updateHeldBadge = () => {
		heldBadge.textContent = String(held.length);
		heldBadge.style.display = held.length > 0 ? "flex" : "none";
	};

	const positionHeld = () => {
		for (const item of held) {
			const box = viewportBox(
				item.target.getBoundingClientRect(),
				win,
				outlineInset,
			);
			item.node.style.left = `${box.left}px`;
			item.node.style.top = `${box.top}px`;
			item.node.style.width = `${box.width}px`;
			item.node.style.height = `${box.height}px`;
		}
	};

	const discardHeld = () => {
		for (const item of held.splice(0)) {
			item.node.remove();
			item.target.removeAttribute(annotationIdAttribute);
		}
		updateHeldBadge();
	};
	heldBadge.addEventListener("click", (event) => {
		event.stopPropagation();
		discardHeld();
	});

	// Shift+Send: keep the comment and stay in picking mode so several
	// elements can be described before anything goes to the agent.
	const holdAnnotation = async (session: PopupSession, comment: string) => {
		const annotation = await createAnnotation(session, comment);
		const node = el(doc, "div", "held-outline", { "aria-hidden": "true" });
		shadow.append(node);
		held.push({ annotation, target: session.target, node });
		positionHeld();
		updateHeldBadge();
		dismissHint();
	};

	// Plain Send: this comment plus anything held goes as one submission.
	const submitAnnotation = async (session: PopupSession, comment: string) => {
		const annotations = [
			...held.map((item) => item.annotation),
			await createAnnotation(session, comment),
		];
		const targets = [...held.map((item) => item.target), session.target];
		for (const item of held.splice(0)) {
			item.node.remove();
		}
		updateHeldBadge();
		const page = pageInfo();
		options.onSubmit({ page, annotations });
		for (const target of targets) {
			flashSent(target);
		}
		// A first comment proves the hint has done its job.
		dismissHint();
		// Hold a quiet ring on the elements until the dashboard reports the
		// agent working on them, so the send and the shimmer read as one
		// continuous state rather than two events with a gap between.
		for (const annotation of annotations) {
			highlights.markPending({
				id: annotation.id,
				selector: annotation.element.selector,
				url: page.url,
			});
		}
	};

	// A one-shot pulse of the outline plus a "Sent" chip where the badge
	// sits, so the user sees the comment went without looking away.
	const flashSent = (target: Element) => {
		const box = viewportBox(target.getBoundingClientRect(), win, outlineInset);
		const flash = el(doc, "div", "sent-flash", { "aria-hidden": "true" });
		flash.style.left = `${box.left}px`;
		flash.style.top = `${box.top}px`;
		flash.style.width = `${box.width}px`;
		flash.style.height = `${box.height}px`;
		flash.classList.toggle("at-top", box.clampedTop);
		flash.classList.toggle("at-right", box.clampedRight);
		flash.append(statusChip(doc, "Sent"));
		shadow.append(flash);
		win.setTimeout(() => flash.remove(), sentFlashMs);
	};

	const isOwnNode = (node: EventTarget | null): boolean =>
		node instanceof Node && (node === host || host.contains(node));

	// Events from inside the shadow root reach document listeners retargeted
	// to the host, and synthetic dispatchers may not expose a composed path.
	const isOwnEvent = (event: Event): boolean =>
		isOwnNode(event.target) || isOwnNode(event.composedPath()[0] ?? null);

	const targetFromEvent = (event: Event): Element | null => {
		if (isOwnEvent(event)) {
			return null;
		}
		const first = event.composedPath()[0] ?? event.target;
		if (!(first instanceof Node)) {
			return null;
		}
		const element = first instanceof Element ? first : first.parentElement;
		if (!element || element === doc.documentElement || element === doc.body) {
			return null;
		}
		return element;
	};

	const positionHighlight = () => {
		const target = popup ? popupTarget : hovered;
		if (!target || !picking) {
			highlight.style.display = "none";
			return;
		}
		// Drawn just outside the element so the dashed border does not sit
		// on top of its edges, and kept within the viewport so the outline
		// of an oversized element is still visible.
		const box = viewportBox(target.getBoundingClientRect(), win, outlineInset);
		highlight.style.display = "block";
		highlight.style.left = `${box.left}px`;
		highlight.style.top = `${box.top}px`;
		highlight.style.width = `${box.width}px`;
		highlight.style.height = `${box.height}px`;
		highlight.classList.toggle("at-top", box.clampedTop);
		highlight.classList.toggle("at-right", box.clampedRight);
		highlightLabel.textContent = shortLabel(target);
	};

	const scheduleLayout = () => {
		if (frame !== 0) {
			return;
		}
		frame = win.requestAnimationFrame(() => {
			frame = 0;
			positionHighlight();
			positionHeld();
		});
	};

	const closePopup = () => {
		popup?.remove();
		popup = null;
		popupTarget = null;
		scheduleLayout();
	};

	const openPopup = (session: PopupSession) => {
		closePopup();
		const node = el(doc, "div", "popup", {
			role: "dialog",
			"aria-label": "Annotate element",
		});
		const title = el(doc, "div", "popup-title");
		title.textContent = "Comment";
		const target = el(doc, "span", "popup-target");
		target.textContent = `(${shortLabel(session.target)})`;
		title.append(" ", target);
		const textarea = el(doc, "textarea", undefined, {
			placeholder: "What should change here?",
			"aria-label": "Annotation comment",
			rows: "3",
		});
		const actions = el(doc, "div", "popup-actions");
		const cancel = el(doc, "button", "button outline", { type: "button" });
		cancel.textContent = "Cancel";
		cancel.addEventListener("click", closePopup);
		const send = el(doc, "button", "button", {
			type: "button",
			disabled: "",
		});
		send.setAttribute(
			"data-tip",
			"Shift+click to hold this comment and pick more",
		);
		const setSendLabel = (hold: boolean) => {
			send.innerHTML = hold ? plusIcon : sendIcon;
			send.append(
				hold ? "Add" : held.length > 0 ? `Send ${held.length + 1}` : "Send",
			);
		};
		setSendLabel(false);
		const submit = (hold: boolean) => {
			const comment = textarea.value.trim();
			if (!comment) {
				textarea.focus();
				return;
			}
			if (hold) {
				void holdAnnotation(session, comment);
			} else {
				void submitAnnotation(session, comment);
			}
			closePopup();
		};
		send.addEventListener("click", (event) => submit(event.shiftKey));
		textarea.addEventListener("input", () => {
			send.disabled = textarea.value.trim() === "";
		});
		// Holding Shift previews what the button will do.
		const onShift = (event: KeyboardEvent) => setSendLabel(event.shiftKey);
		node.addEventListener("keydown", onShift);
		node.addEventListener("keyup", onShift);
		actions.append(cancel, send);
		textarea.addEventListener("keydown", (event) => {
			if (event.key === "Enter" && !event.shiftKey) {
				event.preventDefault();
				submit(false);
			}
		});
		node.append(title, textarea, actions);

		const rect = session.target.getBoundingClientRect();
		const width = 384;
		const left = Math.max(8, Math.min(rect.left, win.innerWidth - width - 8));
		const belowTop = rect.bottom + 8;
		const top =
			belowTop + 220 > win.innerHeight ? Math.max(8, rect.top - 228) : belowTop;
		node.style.left = `${left}px`;
		node.style.top = `${top}px`;
		shadow.append(node);
		popup = node;
		popupTarget = session.target;
		positionHighlight();
		textarea.focus();
	};

	const onPointerMove = (event: MouseEvent) => {
		if (popup) {
			return;
		}
		const next = targetFromEvent(event);
		if (next !== hovered) {
			hovered = next;
			scheduleLayout();
		}
	};

	// Keep the page's own handlers from reacting to picks, but leave the
	// default actions alone so text selection still works and browsers do
	// not suppress the click that follows a cancelled pointerdown.
	const swallow = (event: Event) => {
		if (isOwnEvent(event)) {
			return;
		}
		event.stopImmediatePropagation();
	};

	const onClick = (event: MouseEvent) => {
		if (popup) {
			if (!isOwnEvent(event)) {
				event.preventDefault();
				event.stopImmediatePropagation();
				closePopup();
			}
			return;
		}
		const target = targetFromEvent(event);
		if (!target) {
			return;
		}
		event.preventDefault();
		event.stopImmediatePropagation();
		const selection = win.getSelection()?.toString().trim();
		openPopup({
			target,
			selectedText: selection ? selection.slice(0, 500) : undefined,
		});
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
		if (popup) {
			closePopup();
		} else {
			setPicking(false);
		}
	};

	const interceptedEvents = [
		"mousedown",
		"mouseup",
		"pointerdown",
		"pointerup",
	] as const;

	const setPicking = (next: boolean, withHint = false) => {
		if (next && withHint) {
			hintWanted = true;
		}
		showHint(next && hintWanted);
		if (next === picking) {
			return;
		}
		picking = next;
		toolbar.style.display = next ? "flex" : "none";
		if (next) {
			doc.head.append(cursorStyle);
			doc.addEventListener("mousemove", onPointerMove, true);
			doc.addEventListener("click", onClick, true);
			doc.addEventListener("keydown", onKeyDown, true);
			for (const type of interceptedEvents) {
				doc.addEventListener(type, swallow, true);
			}
		} else {
			cursorStyle.remove();
			doc.removeEventListener("mousemove", onPointerMove, true);
			doc.removeEventListener("click", onClick, true);
			doc.removeEventListener("keydown", onKeyDown, true);
			for (const type of interceptedEvents) {
				doc.removeEventListener(type, swallow, true);
			}
			hovered = null;
			closePopup();
		}
		scheduleLayout();
		notify();
	};

	pickButton.addEventListener("click", () => setPicking(false));
	win.addEventListener("scroll", scheduleLayout, true);
	win.addEventListener("resize", scheduleLayout);
	const stopCarryingMarker = carryMarkerAcrossNavigation(doc, win);
	notify();

	return {
		setPicking,
		setHighlights: highlights.set,
		resolveHighlights: highlights.resolve,
		getState: () => ({ picking }),
		destroy: () => {
			setPicking(false);
			discardHeld();
			highlights.destroy();
			win.removeEventListener("scroll", scheduleLayout, true);
			win.removeEventListener("resize", scheduleLayout);
			stopCarryingMarker();
			if (frame !== 0) {
				win.cancelAnimationFrame(frame);
			}
			host.remove();
		},
	};
}
