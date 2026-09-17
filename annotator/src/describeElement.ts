import type { AnnotatedElement } from "./protocol";
import { describeReactOwner } from "./reactFiber";

const maxSelectorDepth = 6;
const maxClassesPerSegment = 2;
const maxTextLength = 120;
const maxAttributeLength = 120;

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

function cssIdentifier(value: string): string {
	// CSS.escape is universal in browsers but missing from jsdom.
	return typeof CSS !== "undefined" && typeof CSS.escape === "function"
		? CSS.escape(value)
		: value.replaceAll(/[^\w-]/g, (char) => `\\${char}`);
}

function segmentFor(element: Element): string {
	const tag = element.tagName.toLowerCase();
	const classes = Array.from(element.classList)
		.filter(isStableClass)
		.slice(0, maxClassesPerSegment)
		.map((name) => `.${cssIdentifier(name)}`)
		.join("");
	let segment = `${tag}${classes}`;
	const parent = element.parentElement;
	if (!parent) {
		return segment;
	}
	const siblings = Array.from(parent.children).filter((child) =>
		child.matches(segment),
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
 * Builds the shortest selector that uniquely identifies the element,
 * preferring ids and test ids, then stable class names, then positional
 * fallbacks. The result is meant for a human or an agent to grep, not for
 * runtime querying, so readability wins over exhaustiveness.
 */
export function buildSelector(element: Element): string {
	const root = element.ownerDocument;
	if (element.id && isUnique(root, `#${cssIdentifier(element.id)}`)) {
		return `#${cssIdentifier(element.id)}`;
	}
	const testId = element.getAttribute("data-testid");
	if (testId) {
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
		if (current.id && isUnique(root, `#${cssIdentifier(current.id)}`)) {
			segments.unshift(`#${cssIdentifier(current.id)}`);
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

function truncate(value: string, limit: number): string {
	const collapsed = value.replaceAll(/\s+/g, " ").trim();
	return collapsed.length > limit
		? `${collapsed.slice(0, limit - 1)}…`
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
		if (!allowedAttributes.has(lower) && !lower.startsWith("aria-")) {
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
// copy, so it is never captured. Selected text is exempt because the user
// highlighted it deliberately.
const userContentSelector =
	"input, textarea, select, [contenteditable]:not([contenteditable=false])";

function visibleText(element: Element): string {
	if (element.closest(userContentSelector)) {
		return "";
	}
	const walker = element.ownerDocument.createTreeWalker(
		element,
		NodeFilter.SHOW_TEXT,
		{
			acceptNode: (node) =>
				node.parentElement?.closest(userContentSelector)
					? NodeFilter.FILTER_REJECT
					: NodeFilter.FILTER_ACCEPT,
		},
	);
	const parts: string[] = [];
	for (let node = walker.nextNode(); node; node = walker.nextNode()) {
		parts.push(node.textContent ?? "");
	}
	return parts.join(" ");
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
	const role = element.getAttribute("role") ?? undefined;
	const ariaLabel = element.getAttribute("aria-label") ?? undefined;
	const testId = element.getAttribute("data-testid") ?? undefined;

	return {
		tag: element.tagName.toLowerCase(),
		selector: buildSelector(element),
		id: element.id || undefined,
		testId,
		classes: Array.from(element.classList),
		role,
		ariaLabel,
		text: text || undefined,
		openingTag,
		rect: {
			x: Math.round(rect.x + window.scrollX),
			y: Math.round(rect.y + window.scrollY),
			width: Math.round(rect.width),
			height: Math.round(rect.height),
		},
		reactComponents: react?.components.length ? react.components : undefined,
		sourceLocation: react?.sourceLocation,
	};
}
