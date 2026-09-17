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

export type HostToAnnotatorMessage = {
	type: "coder-annotator:set-picking";
	picking: boolean;
};

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

const maxAnnotations = 50;
const maxFieldLength = 2000;
const maxClasses = 50;

function optionalString(value: unknown): string | undefined {
	return typeof value === "string" ? value.slice(0, maxFieldLength) : undefined;
}

function requiredString(value: unknown): string {
	return optionalString(value) ?? "";
}

function finiteNumber(value: unknown): number {
	return typeof value === "number" && Number.isFinite(value) ? value : 0;
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

function parseElement(value: unknown): AnnotatedElement | undefined {
	if (!isRecord(value)) {
		return undefined;
	}
	const classes = Array.isArray(value.classes)
		? value.classes.slice(0, maxClasses).map(requiredString)
		: [];
	const reactComponents = Array.isArray(value.reactComponents)
		? value.reactComponents.slice(0, 10).map(requiredString)
		: undefined;
	return {
		tag: requiredString(value.tag),
		selector: requiredString(value.selector),
		id: optionalString(value.id),
		testId: optionalString(value.testId),
		classes,
		role: optionalString(value.role),
		ariaLabel: optionalString(value.ariaLabel),
		text: optionalString(value.text),
		openingTag: requiredString(value.openingTag),
		rect: parseRect(value.rect),
		reactComponents,
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
	const annotations: Annotation[] = [];
	for (const item of value.annotations.slice(0, maxAnnotations)) {
		if (!isRecord(item)) {
			continue;
		}
		const element = parseElement(item.element);
		if (!element) {
			continue;
		}
		annotations.push({
			id: requiredString(item.id),
			comment: requiredString(item.comment),
			element,
			selectedText: optionalString(item.selectedText),
		});
	}
	if (annotations.length === 0) {
		return undefined;
	}
	const viewport = isRecord(value.page.viewport) ? value.page.viewport : {};
	return {
		page: {
			url: requiredString(value.page.url),
			title: requiredString(value.page.title),
			viewport: {
				width: finiteNumber(viewport.width),
				height: finiteNumber(viewport.height),
			},
		},
		annotations,
	};
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
		default:
			return false;
	}
}
