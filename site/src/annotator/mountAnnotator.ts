import { describeElement } from "./describeElement";
import type { Annotation, AnnotationSubmission } from "./protocol";
import { annotatorStyles, pickingCursorStyles } from "./styles";

interface AnnotatorState {
	picking: boolean;
	count: number;
}

interface AnnotatorHandle {
	setPicking(picking: boolean): void;
	clear(): void;
	getState(): AnnotatorState;
	destroy(): void;
}

interface MountAnnotatorOptions {
	document: Document;
	onSubmit(submission: AnnotationSubmission): void;
	onStateChange?(state: AnnotatorState): void;
}

interface PlacedAnnotation extends Annotation {
	target: Element;
	pin: HTMLButtonElement;
}

interface PopupSession {
	target: Element;
	existing?: PlacedAnnotation;
	selectedText?: string;
}

export const annotatorHostId = "coder-annotator-host";

const pointerIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M14 4.1 12 6"/><path d="m5.1 8-2.9-.8"/><path d="m6 12-1.9 2"/><path d="M7.2 2.2 8 5.1"/><path d="M9.037 9.69a.498.498 0 0 1 .653-.653l11 4.5a.5.5 0 0 1-.074.949l-4.349 1.041a1 1 0 0 0-.74.739l-1.04 4.35a.5.5 0 0 1-.95.074z"/></svg>';
const paperclipIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M13.234 20.252 21 12.3"/><path d="m16 6-8.414 8.586a2 2 0 0 0 0 2.828 2 2 0 0 0 2.828 0l8.414-8.586a4 4 0 0 0 0-5.656 4 4 0 0 0-5.656 0l-8.415 8.585a6 6 0 1 0 8.486 8.486"/></svg>';
const xIcon =
	'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>';

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
	doc.getElementById(annotatorHostId)?.remove();

	const host = el(doc, "div");
	host.id = annotatorHostId;
	const shadow = host.attachShadow({ mode: "open" });
	const style = el(doc, "style");
	style.textContent = annotatorStyles;
	shadow.append(style);

	const toolbar = el(doc, "div", "toolbar", {
		role: "toolbar",
		"aria-label": "UI annotations",
	});
	// Icon buttons with CSS tooltips, matching the dashboard's subtle icon
	// Button and Tooltip primitives.
	const pickButton = el(doc, "button", "icon-button", {
		type: "button",
		"aria-pressed": "false",
		"aria-label": "Annotate elements",
		"data-tip": "Click an element to annotate it",
	});
	pickButton.innerHTML = pointerIcon;
	const countBadge = el(doc, "span", "badge");
	const sendButton = el(doc, "button", "icon-button", {
		type: "button",
		"aria-label": "Attach annotations to your chat message",
		"data-tip": "Attach to chat message",
	});
	sendButton.innerHTML = paperclipIcon;
	const clearButton = el(doc, "button", "icon-button", {
		type: "button",
		"aria-label": "Clear annotations",
		"data-tip": "Clear annotations",
	});
	clearButton.innerHTML = xIcon;
	toolbar.append(pickButton, countBadge, sendButton, clearButton);

	const highlight = el(doc, "div", "highlight", { "aria-hidden": "true" });
	const highlightLabel = el(doc, "span", "highlight-label");
	highlight.append(highlightLabel);

	const pins = el(doc, "div", "pins");
	shadow.append(toolbar, highlight, pins);
	doc.body.append(host);

	const cursorStyle = el(doc, "style");
	cursorStyle.textContent = pickingCursorStyles;

	const annotations: PlacedAnnotation[] = [];
	let picking = false;
	let hovered: Element | null = null;
	let popup: HTMLDivElement | null = null;
	let frame = 0;

	const notify = () => {
		countBadge.textContent = String(annotations.length);
		countBadge.hidden = annotations.length === 0;
		sendButton.disabled = annotations.length === 0;
		clearButton.hidden = annotations.length === 0;
		options.onStateChange?.({ picking, count: annotations.length });
	};

	const pageInfo = () => ({
		url: win.location.href,
		title: doc.title,
		viewport: { width: win.innerWidth, height: win.innerHeight },
	});

	const toAnnotation = (placed: PlacedAnnotation): Annotation => ({
		id: placed.id,
		comment: placed.comment,
		selectedText: placed.selectedText,
		// Re-measure so positions reflect the page at send time.
		element: describeElement(placed.target),
	});

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
		if (!hovered || !picking || popup) {
			highlight.style.display = "none";
			return;
		}
		const rect = hovered.getBoundingClientRect();
		highlight.style.display = "block";
		highlight.style.left = `${rect.left}px`;
		highlight.style.top = `${rect.top}px`;
		highlight.style.width = `${rect.width}px`;
		highlight.style.height = `${rect.height}px`;
		highlightLabel.textContent = shortLabel(hovered);
	};

	const positionPins = () => {
		for (const annotation of annotations) {
			const rect = annotation.target.getBoundingClientRect();
			const visible = rect.width > 0 || rect.height > 0;
			annotation.pin.style.display = visible ? "flex" : "none";
			annotation.pin.style.left = `${rect.right}px`;
			annotation.pin.style.top = `${rect.top}px`;
		}
	};

	const scheduleLayout = () => {
		if (frame !== 0) {
			return;
		}
		frame = win.requestAnimationFrame(() => {
			frame = 0;
			positionHighlight();
			positionPins();
		});
	};

	const closePopup = () => {
		popup?.remove();
		popup = null;
		scheduleLayout();
	};

	const removeAnnotation = (annotation: PlacedAnnotation) => {
		const index = annotations.indexOf(annotation);
		if (index === -1) {
			return;
		}
		annotations.splice(index, 1);
		annotation.pin.remove();
		annotations.forEach((remaining, i) => {
			remaining.pin.textContent = String(i + 1);
		});
		notify();
	};

	const commitAnnotation = (session: PopupSession, comment: string) => {
		if (session.existing) {
			session.existing.comment = comment;
			session.existing.pin.title = comment;
			return;
		}
		const pin = el(doc, "button", "pin", {
			type: "button",
			title: comment,
			"aria-label": `Annotation ${annotations.length + 1}: ${comment}`,
		});
		pin.textContent = String(annotations.length + 1);
		const placed: PlacedAnnotation = {
			id: crypto.randomUUID(),
			comment,
			element: describeElement(session.target),
			selectedText: session.selectedText,
			target: session.target,
			pin,
		};
		pin.addEventListener("click", (event) => {
			event.stopPropagation();
			openPopup({ target: placed.target, existing: placed });
		});
		pins.append(pin);
		annotations.push(placed);
		notify();
	};

	const openPopup = (session: PopupSession) => {
		closePopup();
		const node = el(doc, "div", "popup", {
			role: "dialog",
			"aria-label": "Annotate element",
		});
		const target = el(doc, "div", "popup-target");
		target.textContent = shortLabel(session.target);
		const textarea = el(doc, "textarea", undefined, {
			placeholder: "What should change here?",
			"aria-label": "Annotation comment",
			rows: "3",
		});
		textarea.value = session.existing?.comment ?? "";
		const actions = el(doc, "div", "popup-actions");
		const hint = el(doc, "span", "hint");
		hint.textContent = "Enter to save, Esc or click away to dismiss";
		const spacer = el(doc, "span", "spacer");
		const save = el(doc, "button", "button", { type: "button" });
		save.textContent = session.existing ? "Update" : "Add";
		const submit = () => {
			const comment = textarea.value.trim();
			if (!comment) {
				textarea.focus();
				return;
			}
			commitAnnotation(session, comment);
			closePopup();
		};
		save.addEventListener("click", submit);
		actions.append(hint, spacer);
		if (session.existing) {
			const remove = el(doc, "button", "button destructive", {
				type: "button",
			});
			remove.textContent = "Delete";
			remove.addEventListener("click", () => {
				if (session.existing) {
					removeAnnotation(session.existing);
				}
				closePopup();
			});
			actions.append(remove);
		}
		actions.append(save);
		textarea.addEventListener("keydown", (event) => {
			if (event.key === "Escape") {
				event.preventDefault();
				closePopup();
			} else if (event.key === "Enter" && !event.shiftKey) {
				event.preventDefault();
				submit();
			}
		});
		node.append(target, textarea, actions);

		const rect = session.target.getBoundingClientRect();
		const width = 320;
		const left = Math.max(8, Math.min(rect.left, win.innerWidth - width - 8));
		const belowTop = rect.bottom + 8;
		const top =
			belowTop + 160 > win.innerHeight ? Math.max(8, rect.top - 168) : belowTop;
		node.style.left = `${left}px`;
		node.style.top = `${top}px`;
		shadow.append(node);
		popup = node;
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
		if (popup || isOwnEvent(event)) {
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

	const onKeyDown = (event: KeyboardEvent) => {
		if (event.key === "Escape" && !popup) {
			setPicking(false);
		}
	};

	const interceptedEvents = [
		"mousedown",
		"mouseup",
		"pointerdown",
		"pointerup",
	] as const;

	const setPicking = (next: boolean) => {
		if (next === picking) {
			return;
		}
		picking = next;
		pickButton.setAttribute("aria-pressed", String(next));
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

	const clear = () => {
		for (const annotation of annotations.splice(0)) {
			annotation.pin.remove();
		}
		closePopup();
		notify();
	};

	pickButton.addEventListener("click", () => setPicking(!picking));
	clearButton.addEventListener("click", clear);
	sendButton.addEventListener("click", () => {
		if (annotations.length === 0) {
			return;
		}
		options.onSubmit({
			page: pageInfo(),
			annotations: annotations.map(toAnnotation),
		});
		clear();
		setPicking(false);
	});

	win.addEventListener("scroll", scheduleLayout, true);
	win.addEventListener("resize", scheduleLayout);
	notify();

	return {
		setPicking,
		clear,
		getState: () => ({ picking, count: annotations.length }),
		destroy: () => {
			setPicking(false);
			win.removeEventListener("scroll", scheduleLayout, true);
			win.removeEventListener("resize", scheduleLayout);
			if (frame !== 0) {
				win.cancelAnimationFrame(frame);
			}
			host.remove();
		},
	};
}
