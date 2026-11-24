/** Bounded element context for locating the selected UI in source code. */
export type AnnotatedElement = {
	tag: string;
	selector: string;
	id?: string;
	testId?: string;
	classes: string[];
	role?: string;
	ariaLabel?: string;
	text?: string;
	// Rebuilt from an attribute allowlist, never copied from raw page markup.
	openingTag: string;
	rect: { x: number; y: number; width: number; height: number };
	reactComponents?: string[];
	sourceLocation?: string;
};

/** A user-authored comment and its page context, sent on each submission. */
export type AnnotationSubmission = {
	page: {
		url: string;
		title: string;
		viewport: { width: number; height: number };
	};
	annotations: {
		id: string;
		comment: string;
		element: AnnotatedElement;
		// Explicitly selected by the user, unlike automatically captured text.
		selectedText?: string;
	}[];
};

/** Lifecycle, picking state, and submissions sent to the embedding dashboard. */
export type AnnotatorToHostMessage =
	| { type: "coder-annotator:ready" }
	| { type: "coder-annotator:state"; picking: boolean }
	| ({ type: "coder-annotator:submit" } & AnnotationSubmission);

type HostToAnnotatorMessage = {
	type: "coder-annotator:set-picking";
	picking: boolean;
};

/** Limit page-controlled strings before cloning them into the dashboard. */
export const maxFieldLength = 300;

/** Accept only the dashboard command supported by the overlay. */
export function isHostToAnnotatorMessage(
	value: unknown,
): value is HostToAnnotatorMessage {
	return (
		typeof value === "object" &&
		value !== null &&
		"type" in value &&
		value.type === "coder-annotator:set-picking" &&
		"picking" in value &&
		typeof value.picking === "boolean"
	);
}
