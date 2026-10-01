/**
 * Whether an event started inside the overlay's own host element. Events
 * from inside the shadow root reach window and document listeners
 * retargeted to the host, and synthetic dispatchers may not expose a
 * composed path, so both are checked.
 */
export function isOwnEvent(host: Element, event: Event): boolean {
	const isOwnNode = (node: EventTarget | null | undefined) =>
		node instanceof Node && (node === host || host.contains(node));
	return isOwnNode(event.target) || isOwnNode(event.composedPath()[0]);
}

/**
 * The page element a pointer event is about, or null when the event is
 * the overlay's own or lands on the document roots, which are not worth
 * annotating.
 */
export function pickTarget(host: Element, event: Event): Element | null {
	if (isOwnEvent(host, event)) {
		return null;
	}
	const first = event.composedPath()[0] ?? event.target;
	if (!(first instanceof Node)) {
		return null;
	}
	const element = first instanceof Element ? first : first.parentElement;
	const doc = host.ownerDocument;
	if (!element || element === doc.documentElement || element === doc.body) {
		return null;
	}
	return element;
}
