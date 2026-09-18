/**
 * Message contract between the annotator overlay injected into a proxied
 * workspace app and the dashboard that embeds it in an iframe. Both sides
 * import this file, so keep it free of DOM or React imports.
 */

/**
 * Query parameter the dashboard appends to the preview URL. The app proxy
 * strips it before forwarding the request and injects the overlay script
 * into the HTML response. It is request-only: no cookie or other state.
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
	// Opening tag rebuilt from an attribute allowlist; never raw markup.
	openingTag: string;
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
	| { type: "coder-annotator:state"; picking: boolean }
	| ({ type: "coder-annotator:submit" } & AnnotationSubmission);

/**
 * Attribute the overlay stamps on an element when a comment about it is
 * sent, valued with the annotation id. Highlights look the element up by
 * it first, so a same-shaped element on another page is not mistaken for
 * the annotated one.
 */
export const annotationIdAttribute = "data-coder-annotation-id";

export interface HighlightItem {
	id: string;
	selector: string;
	// Page the annotation was made on. The selector is only trusted as a
	// fallback while the preview is still on that page.
	url: string;
}

export type HostToAnnotatorMessage =
	| { type: "coder-annotator:set-picking"; picking: boolean }
	// Marks previously annotated elements while the agent works on them.
	| { type: "coder-annotator:highlight"; items: HighlightItem[] }
	| { type: "coder-annotator:clear-highlights" };

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

// The overlay sends one annotation per save; a handful is plenty of slack.
const maxAnnotations = 5;
const maxCommentLength = 2000;
const maxFieldLength = 300;
const maxClasses = 20;
const maxComponents = 10;
// Coordinates well past any real screen are meaningless and only make
// the output longer.
const maxCoordinate = 100_000;
// Ceiling for every string in one submission put together, so a flood of
// maximal fields cannot turn into a multi-megabyte chat message.
const maxSubmissionLength = 16_000;

function optionalString(
	value: unknown,
	limit = maxFieldLength,
): string | undefined {
	return typeof value === "string" ? value.slice(0, limit) : undefined;
}

function finiteNumber(value: unknown): number {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return 0;
	}
	return Math.max(-maxCoordinate, Math.min(maxCoordinate, Math.round(value)));
}

function stringArray(value: unknown, limit: number): string[] | undefined {
	if (!Array.isArray(value)) {
		return undefined;
	}
	return value
		.slice(0, limit)
		.filter((item): item is string => typeof item === "string")
		.map((item) => item.slice(0, maxFieldLength));
}

// The page value is reference data for the agent, so only the origin and
// path survive: query strings and fragments routinely carry tokens.
function pageLocation(value: unknown): string | undefined {
	if (typeof value !== "string") {
		return undefined;
	}
	try {
		const url = new URL(value);
		if (url.protocol !== "http:" && url.protocol !== "https:") {
			return undefined;
		}
		return `${url.origin}${url.pathname}`.slice(0, maxFieldLength);
	} catch {
		return undefined;
	}
}

function submissionLength(submission: AnnotationSubmission): number {
	return JSON.stringify(submission).length;
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null;
}

function parseRect(value: unknown): AnnotatedElement["rect"] {
	const rect = isRecord(value) ? value : {};
	return {
		x: finiteNumber(rect.x),
		y: finiteNumber(rect.y),
		width: finiteNumber(rect.width),
		height: finiteNumber(rect.height),
	};
}

// Elements missing the fields that identify them are malformed rather
// than merely sparse, so the whole annotation is dropped.
function parseElement(value: unknown): AnnotatedElement | undefined {
	if (!isRecord(value)) {
		return undefined;
	}
	const tag = optionalString(value.tag);
	const selector = optionalString(value.selector);
	const openingTag = optionalString(value.openingTag);
	if (!tag || !selector || !openingTag) {
		return undefined;
	}
	return {
		tag,
		selector,
		id: optionalString(value.id),
		testId: optionalString(value.testId),
		classes: stringArray(value.classes, maxClasses) ?? [],
		role: optionalString(value.role),
		ariaLabel: optionalString(value.ariaLabel),
		text: optionalString(value.text),
		openingTag,
		rect: parseRect(value.rect),
		reactComponents: stringArray(value.reactComponents, maxComponents),
		sourceLocation: optionalString(value.sourceLocation),
	};
}

/**
 * Validates and bounds a submission received over postMessage. The frame
 * is a third-party app, so every field is treated as untrusted input:
 * unknown shapes are rejected and strings and arrays are truncated.
 */
function parseAnnotationSubmission(
	value: unknown,
): AnnotationSubmission | undefined {
	if (
		!isRecord(value) ||
		!isRecord(value.page) ||
		!Array.isArray(value.annotations)
	) {
		return undefined;
	}
	const url = pageLocation(value.page.url);
	if (!url) {
		return undefined;
	}
	const annotations: Annotation[] = [];
	for (const item of value.annotations.slice(0, maxAnnotations)) {
		if (!isRecord(item)) {
			continue;
		}
		const element = parseElement(item.element);
		const id = optionalString(item.id);
		if (!element || !id) {
			continue;
		}
		annotations.push({
			id,
			comment: optionalString(item.comment, maxCommentLength) ?? "",
			element,
			selectedText: optionalString(item.selectedText),
		});
	}
	if (annotations.length === 0) {
		return undefined;
	}
	const viewport = isRecord(value.page.viewport) ? value.page.viewport : {};
	const submission: AnnotationSubmission = {
		page: {
			url,
			title: optionalString(value.page.title) ?? "",
			viewport: {
				width: finiteNumber(viewport.width),
				height: finiteNumber(viewport.height),
			},
		},
		annotations,
	};
	if (submissionLength(submission) > maxSubmissionLength) {
		return undefined;
	}
	return submission;
}

export function parseAnnotatorToHostMessage(
	value: unknown,
): AnnotatorToHostMessage | undefined {
	if (!hasMessageType(value)) {
		return undefined;
	}
	switch (value.type) {
		case "coder-annotator:ready":
			return { type: value.type };
		case "coder-annotator:state": {
			const state: Record<string, unknown> = value;
			return { type: value.type, picking: state.picking === true };
		}
		case "coder-annotator:submit": {
			const submission = parseAnnotationSubmission(value);
			return submission ? { type: value.type, ...submission } : undefined;
		}
		default:
			return undefined;
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
		case "coder-annotator:clear-highlights":
			return true;
		case "coder-annotator:highlight":
			return "items" in value && Array.isArray(value.items);
		default:
			return false;
	}
}
