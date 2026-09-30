import type { AnnotatedElement } from "./protocol";
import { describeReactOwner, resolveReactSource } from "./reactFiber";

const maxSelectorDepth = 6;
const maxClassesPerSegment = 2;
const maxTextLength = 120;
const maxAttributeLength = 120;
const maxClasses = 20;
const maxClassLength = 100;
// Ids and test ids are host-controlled. Past this they are not worth
// escaping and running through the selector engine.
const maxIdLength = 200;

// Attributes that help locate an element in source without carrying page
// state. Everything else (value, data-*, style, handlers, arbitrary
// attributes) is where tokens, form state, and serialized blobs live.
const allowedAttributes = new Set([
	"id",
	"class",
	"data-testid",
	"role",
	"name",
	"type",
	"placeholder",
	"title",
	"alt",
	"href",
	"src",
	"for",
	// Labelling and widget-state ARIA only. aria-valuetext,
	// aria-description and friends can carry user data.
	"aria-label",
	"aria-labelledby",
	"aria-describedby",
	"aria-controls",
	"aria-haspopup",
	"aria-expanded",
	"aria-selected",
	"aria-checked",
	"aria-pressed",
	"aria-current",
	"aria-disabled",
	"aria-hidden",
]);
const urlAttributes = new Set(["href", "src"]);

// Generated class names (CSS modules, styled-components, Tailwind
// arbitrary values) make brittle selectors and are useless for grepping.
const generatedClassPattern =
	/^(css|sc|jsx|emotion|_)-|[0-9a-f]{6,}|[[\]:/!%]|^[a-z]+_[A-Za-z0-9]{5,}$/i;

function isStableClass(name: string): boolean {
	return (
		name.length > 0 && name.length <= 40 && !generatedClassPattern.test(name)
	);
}

function escapeAttribute(value: string): string {
	return value.replaceAll("\\", "\\\\").replaceAll('"', '\\"');
}

function isUnique(root: ParentNode, selector: string): boolean {
	try {
		return root.querySelectorAll(selector).length === 1;
	} catch {
		return false;
	}
}

function matchesSafely(element: Element, selector: string): boolean {
	try {
		return element.matches(selector);
	} catch {
		return false;
	}
}

function segmentFor(element: Element): string {
	const tag = element.tagName.toLowerCase();
	const classes = Array.from(element.classList)
		.filter(isStableClass)
		.slice(0, maxClassesPerSegment)
		.map((name) => `.${CSS.escape(name)}`)
		.join("");
	let segment = `${tag}${classes}`;
	const parent = element.parentElement;
	if (!parent) {
		return segment;
	}
	const siblings = Array.from(parent.children).filter((child) =>
		matchesSafely(child, segment),
	);
	if (siblings.length > 1) {
		const sameTag = Array.from(parent.children).filter(
			(child) => child.tagName === element.tagName,
		);
		segment = `${tag}:nth-of-type(${sameTag.indexOf(element) + 1})`;
	}
	return segment;
}

/**
 * Builds a short selector for the element, preferring ids and test ids,
 * then stable class names, then positional fallbacks. Each step checks
 * for uniqueness but the walk stops at `maxSelectorDepth` segments: the
 * result is meant for a human or an agent to grep, not for runtime
 * querying, so readability wins over exhaustiveness.
 */
export function buildSelector(element: Element): string {
	const root = element.ownerDocument;
	if (element === root.documentElement || element === root.body) {
		return element.tagName.toLowerCase();
	}
	if (usableId(element) && isUnique(root, `#${CSS.escape(element.id)}`)) {
		return `#${CSS.escape(element.id)}`;
	}
	const testId = element.getAttribute("data-testid");
	if (testId && testId.length <= maxIdLength) {
		const selector = `[data-testid="${escapeAttribute(testId)}"]`;
		if (isUnique(root, selector)) {
			return selector;
		}
	}

	const segments: string[] = [];
	let current: Element | null = element;
	while (
		current &&
		current !== root.body &&
		segments.length < maxSelectorDepth
	) {
		if (usableId(current) && isUnique(root, `#${CSS.escape(current.id)}`)) {
			segments.unshift(`#${CSS.escape(current.id)}`);
			break;
		}
		segments.unshift(segmentFor(current));
		// A lone positional segment is unique but unreadable; keep one
		// level of parent context for it.
		const positionalLeaf =
			segments.length === 1 && segments[0].includes(":nth-of-type");
		if (!positionalLeaf && isUnique(root, segments.join(" > "))) {
			break;
		}
		current = current.parentElement;
	}
	return segments.join(" > ");
}

function usableId(element: Element): boolean {
	return element.id !== "" && element.id.length <= maxIdLength;
}

function truncate(value: string, limit: number): string {
	const collapsed = value.replaceAll(/\s+/g, " ").trim();
	return collapsed.length > limit
		? `${collapsed.slice(0, limit - 3)}...`
		: collapsed;
}

function escapeAttributeValue(value: string): string {
	return value
		.replaceAll("&", "&amp;")
		.replaceAll('"', "&quot;")
		.replaceAll("<", "&lt;");
}

function stripUrlSecrets(value: string): string {
	// Query strings and fragments routinely carry tokens; the path is enough
	// to find the source.
	const cut = value.search(/[?#]/);
	return cut === -1 ? value : value.slice(0, cut);
}

/**
 * Rebuilds the element's opening tag from an attribute allowlist so the
 * output is greppable without leaking page state.
 */
export function describeOpeningTag(element: Element): string {
	const parts = [element.tagName.toLowerCase()];
	for (const { name, value } of Array.from(element.attributes)) {
		const lower = name.toLowerCase();
		if (!allowedAttributes.has(lower)) {
			continue;
		}
		const safe = truncate(
			urlAttributes.has(lower) ? stripUrlSecrets(value) : value,
			maxAttributeLength,
		);
		parts.push(`${lower}="${escapeAttributeValue(safe)}"`);
	}
	return `<${parts.join(" ")}>`;
}

// Text inside form controls and editable regions is user state, not UI
// copy, so it is never captured.
const userContentSelector =
	"input, textarea, select, [contenteditable]:not([contenteditable=false])";
// Not rendered, but often full of serialized state or configuration.
const nonVisibleSelector =
	'script, style, template, noscript, [hidden], [aria-hidden="true"]';

// Hidden through CSS rather than markup: collapsed menus and inactive
// tabs. The user never saw that text either.
function hiddenByStyle(element: Element): boolean {
	if (typeof element.checkVisibility === "function") {
		return !element.checkVisibility({ visibilityProperty: true });
	}
	const style = element.ownerDocument.defaultView?.getComputedStyle(element);
	return style?.display === "none" || style?.visibility === "hidden";
}

function visibleText(element: Element): string {
	const excluded = `${userContentSelector}, ${nonVisibleSelector}`;
	// The walker only filters descendants, so the element and its ancestors
	// go through the same rules first.
	if (element.closest(excluded) || hiddenByStyle(element)) {
		return "";
	}
	const walker = element.ownerDocument.createTreeWalker(
		element,
		NodeFilter.SHOW_ELEMENT | NodeFilter.SHOW_TEXT,
		{
			acceptNode: (node) => {
				if (node instanceof Element) {
					// Rejecting the element skips its whole subtree.
					return node.matches(excluded) || hiddenByStyle(node)
						? NodeFilter.FILTER_REJECT
						: NodeFilter.FILTER_SKIP;
				}
				return NodeFilter.FILTER_ACCEPT;
			},
		},
	);
	// Stop once there is more than enough for the truncated result rather
	// than serialising an arbitrarily large subtree.
	let text = "";
	for (let node = walker.nextNode(); node; node = walker.nextNode()) {
		text += ` ${node.textContent ?? ""}`;
		if (text.length > maxTextLength * 4) {
			break;
		}
	}
	return text;
}

/**
 * Captures everything an agent needs to locate the element in source:
 * selector, identifying attributes, visible text, an allowlisted opening
 * tag, geometry, and (when available) the owning React components.
 */
export function describeElement(element: Element): AnnotatedElement {
	const rect = element.getBoundingClientRect();
	const text = truncate(visibleText(element), maxTextLength);
	const react = describeReactOwner(element);
	const openingTag = describeOpeningTag(element);
	const attribute = (name: string) => {
		const value = element.getAttribute(name);
		return value ? truncate(value, maxAttributeLength) : undefined;
	};

	return {
		tag: element.tagName.toLowerCase(),
		selector: buildSelector(element),
		id: element.id ? truncate(element.id, maxIdLength) : undefined,
		testId: attribute("data-testid"),
		classes: Array.from(element.classList)
			.slice(0, maxClasses)
			.map((name) => truncate(name, maxClassLength)),
		role: attribute("role"),
		ariaLabel: attribute("aria-label"),
		text: text || undefined,
		openingTag,
		rect: {
			x: Math.round(rect.x + window.scrollX),
			y: Math.round(rect.y + window.scrollY),
			width: Math.round(rect.width),
			height: Math.round(rect.height),
		},
		reactComponents: react?.components.length ? react.components : undefined,
		reactProps: react?.props,
	};
}

/**
 * `describeElement` plus the React source data that needs source maps.
 * The element is described synchronously first so a slow or missing
 * source map can only cost the extra detail, never the annotation.
 */
export async function describeElementWithSource(
	element: Element,
): Promise<AnnotatedElement> {
	const described = describeElement(element);
	const source = await resolveReactSource(element);
	return source ? { ...described, ...source } : described;
}
