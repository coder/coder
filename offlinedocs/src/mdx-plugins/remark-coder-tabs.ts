// Convert `<div class="tabs">` blocks into Fumadocs `Tabs`/`Tab` components.
// Handles nested tabs, splits bundled OS labels (e.g. `Linux/macOS`) into one
// tab per OS, and emits OS-only sets as `OSTab` and other sets as grouped `Tabs`.

import { isHtml, type MdastNode } from "./mdast";

const TABS_OPEN = /^<div\s+class="tabs"\s*>/i;

// Net `<div>` nesting change contributed by a raw html node's text.
export function divDelta(value: string): number {
	const opens = (value.match(/<div\b/gi) ?? []).length;
	const closes = (value.match(/<\/div>/gi) ?? []).length;
	return opens - closes;
}

function flattenText(node: MdastNode): string {
	if (node.type === "text" || node.type === "inlineCode")
		return node.value ?? "";
	if (node.children) return node.children.map(flattenText).join("");
	return "";
}

// Canonical OS names keyed by their lowercased spellings.
const OS_CANONICAL: Record<string, string> = {
	linux: "Linux",
	macos: "macOS",
	mac: "macOS",
	"mac os": "macOS",
	"mac os x": "macOS",
	osx: "macOS",
	"os x": "macOS",
	darwin: "macOS",
	windows: "Windows",
	win: "Windows",
};

export function canonicalOS(token: string): string | null {
	return OS_CANONICAL[token.trim().toLowerCase()] ?? null;
}

// Canonical OS names if a label is only OS names joined by `/`, `,`, `&`, or `and`; else null.
export function splitOSLabel(label: string): string[] | null {
	const tokens = label
		.split(/\s*(?:\/|,|&|\band\b)\s*/i)
		.map((token) => token.trim())
		.filter(Boolean);
	if (tokens.length === 0) return null;

	const canonical: string[] = [];
	const seen = new Set<string>();
	for (const token of tokens) {
		const os = canonicalOS(token);
		if (!os) return null;
		if (!seen.has(os)) {
			seen.add(os);
			canonical.push(os);
		}
	}
	return canonical;
}

// Build the `items={[...]}` attribute backed by an estree ArrayExpression.
function itemsAttribute(labels: string[]) {
	const raws = labels.map((label) => JSON.stringify(label));
	return {
		type: "mdxJsxAttribute",
		name: "items",
		value: {
			type: "mdxJsxAttributeValueExpression",
			value: `[${raws.join(", ")}]`,
			data: {
				estree: {
					type: "Program",
					sourceType: "module",
					body: [
						{
							type: "ExpressionStatement",
							expression: {
								type: "ArrayExpression",
								elements: labels.map((label, i) => ({
									type: "Literal",
									value: label,
									raw: raws[i],
								})),
							},
						},
					],
				},
			},
		},
	};
}

function stringAttribute(name: string, value: string) {
	return { type: "mdxJsxAttribute", name, value };
}

// A boolean JSX attribute backed by an estree `true` literal.
function trueAttribute(name: string) {
	return {
		type: "mdxJsxAttribute",
		name,
		value: {
			type: "mdxJsxAttributeValueExpression",
			value: "true",
			data: {
				estree: {
					type: "Program",
					sourceType: "module",
					body: [
						{
							type: "ExpressionStatement",
							expression: { type: "Literal", value: true, raw: "true" },
						},
					],
				},
			},
		},
	};
}

// Stable slug identifier so tab sets that share a label set stay in sync.
export function groupIdFor(labels: string[]): string {
	return [...labels]
		.map((label) => label.toLowerCase().trim())
		.sort()
		.join("|")
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
}

type TabDef = { label: string; content: MdastNode[] };

// Expand any bundled OS label into one tab per OS, cloning duplicated content.
function expandOSTabs(tabs: TabDef[]): TabDef[] {
	const out: TabDef[] = [];
	for (const tab of tabs) {
		const parts = splitOSLabel(tab.label);
		if (parts && parts.length > 1) {
			parts.forEach((os, index) => {
				out.push({
					label: os,
					content:
						index === 0
							? tab.content
							: (structuredClone(tab.content) as MdastNode[]),
				});
			});
		} else {
			// Solo OS aliases still emit under their canonical name; `parts` is null for non-OS labels.
			out.push({ label: parts?.[0] ?? tab.label, content: tab.content });
		}
	}
	return out;
}

type Built = { node: MdastNode; pre: MdastNode[]; endIndex: number };

function buildTabs(children: MdastNode[], openIndex: number): Built | null {
	const openNode = children[openIndex];
	if (!isHtml(openNode)) return null;

	// Balance nested `<div>`s to find the matching `</div>`.
	let depth = divDelta(openNode.value);
	let closeIndex = -1;
	for (let j = openIndex + 1; j < children.length; j++) {
		const node = children[j];
		if (isHtml(node)) {
			depth += divDelta(node.value);
			if (depth <= 0) {
				closeIndex = j;
				break;
			}
		}
	}
	if (closeIndex === -1) return null;

	// Convert nested tab blocks before splitting so inner headings do not leak into the outer split.
	const inner = transformList(children.slice(openIndex + 1, closeIndex));

	// The first heading sets the tab-delimiter depth; each heading at that depth starts a new tab.
	let tabDepth: number | null = null;
	const pre: MdastNode[] = [];
	const tabs: TabDef[] = [];
	let current: TabDef | null = null;

	for (const node of inner) {
		if (
			node.type === "heading" &&
			(tabDepth === null || node.depth === tabDepth)
		) {
			if (tabDepth === null) tabDepth = node.depth ?? null;
			current = { label: flattenText(node).trim(), content: [] };
			tabs.push(current);
		} else if (current) {
			current.content.push(node);
		} else {
			pre.push(node);
		}
	}

	if (tabs.length === 0) return null;

	const expanded = expandOSTabs(tabs);
	const labels = expanded.map((tab) => tab.label);
	const isOSSet = labels.every((label) => canonicalOS(label) !== null);

	const attributes: MdastNode[] = [
		itemsAttribute(labels) as unknown as MdastNode,
	];
	if (!isOSSet) {
		attributes.push(
			stringAttribute("groupId", groupIdFor(labels)) as unknown as MdastNode,
		);
		attributes.push(trueAttribute("persist") as unknown as MdastNode);
	}

	const node: MdastNode = {
		type: "mdxJsxFlowElement",
		name: isOSSet ? "OSTab" : "Tabs",
		attributes,
		children: expanded.map((tab) => ({
			type: "mdxJsxFlowElement",
			name: "Tab",
			attributes: [stringAttribute("value", tab.label)],
			children: tab.content,
		})),
	};

	return { node, pre, endIndex: closeIndex };
}

// Convert every `<div class="tabs">` block in a sibling list, recursing into child lists first.
function transformList(nodes: MdastNode[]): MdastNode[] {
	for (const node of nodes) {
		if (node.children) node.children = transformList(node.children);
	}

	const out: MdastNode[] = [];
	for (let i = 0; i < nodes.length; i++) {
		const node = nodes[i];
		if (isHtml(node) && TABS_OPEN.test(node.value.trim())) {
			const built = buildTabs(nodes, i);
			if (built) {
				out.push(...built.pre, built.node);
				i = built.endIndex;
				continue;
			}
		}
		out.push(node);
	}
	return out;
}

export function remarkCoderTabs() {
	return (tree: MdastNode): void => {
		if (tree.children) tree.children = transformList(tree.children);
	};
}
