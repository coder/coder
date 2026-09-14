/**
 * Message contract between the annotator overlay injected into a proxied
 * workspace app and the dashboard that embeds it in an iframe. Both sides
 * import this file, so keep it free of DOM or React imports.
 */

/**
 * Query parameter the dashboard appends to the iframe URL. The app proxy
 * turns it into a host-only cookie and strips it, so client-side
 * navigations inside the preview keep the overlay.
 */
export const annotatorQueryParam = "coder_annotate";

export interface AnnotatedElement {
	tag: string;
	selector: string;
	id?: string;
	testId?: string;
	classes: string[];
	role?: string;
	ariaLabel?: string;
	text?: string;
	html: string;
	rect: { x: number; y: number; width: number; height: number };
	reactComponents?: string[];
	sourceLocation?: string;
}

export interface Annotation {
	id: string;
	comment: string;
	element: AnnotatedElement;
	selectedText?: string;
}

interface AnnotatedPage {
	url: string;
	title: string;
	viewport: { width: number; height: number };
}

export interface AnnotationSubmission {
	page: AnnotatedPage;
	annotations: Annotation[];
}

export type AnnotatorToHostMessage =
	| { type: "coder-annotator:ready" }
	| { type: "coder-annotator:state"; picking: boolean; count: number }
	| ({ type: "coder-annotator:submit" } & AnnotationSubmission);

export type HostToAnnotatorMessage =
	| { type: "coder-annotator:set-picking"; picking: boolean }
	| { type: "coder-annotator:clear" };

const messagePrefix = "coder-annotator:";

function hasMessageType(value: unknown): value is { type: string } {
	return (
		typeof value === "object" &&
		value !== null &&
		"type" in value &&
		typeof value.type === "string" &&
		value.type.startsWith(messagePrefix)
	);
}

export function isAnnotatorToHostMessage(
	value: unknown,
): value is AnnotatorToHostMessage {
	if (!hasMessageType(value)) {
		return false;
	}
	switch (value.type) {
		case "coder-annotator:ready":
			return true;
		case "coder-annotator:state":
			return "picking" in value && "count" in value;
		case "coder-annotator:submit":
			return "annotations" in value && "page" in value;
		default:
			return false;
	}
}

export function isHostToAnnotatorMessage(
	value: unknown,
): value is HostToAnnotatorMessage {
	if (!hasMessageType(value)) {
		return false;
	}
	switch (value.type) {
		case "coder-annotator:set-picking":
			return "picking" in value && typeof value.picking === "boolean";
		case "coder-annotator:clear":
			return true;
		default:
			return false;
	}
}
