// Convert `<div class="tabs">` blocks into Fumadocs `Tabs`/`Tab` components.
//
// The coder/coder corpus authors tabs as a raw HTML wrapper whose immediate
// headings are the tab labels:
//
//   <div class="tabs">
//
//   ### UI
//   ...content...
//
//   ### CLI
//   ...content...
//
//   </div>
//
// In the plain-Markdown (.md) pipeline those `<div ...>`/`</div>` lines arrive
// as raw `html` mdast nodes with the headings and body as normal Markdown nodes
// between them. This plugin finds each wrapper, splits the inner nodes on the
// first heading depth, and emits one tab per heading so it renders through the
// component map.
//
// On top of that base conversion it applies three authoring upgrades:
//
//   * Nested tabs. Inner `<div class="tabs">` blocks are converted before the
//     outer split, so a distro picker inside a "Linux" tab stays nested instead
//     of being flattened into sibling tabs (e.g. install/uninstall).
//   * Split bundled OS labels. A heading whose label is only operating-system
//     names joined by `/`, `,`, `&`, or `and` (e.g. `Linux/macOS`) becomes one
//     tab per OS, with the content duplicated, so OS detection can target a
//     single operating system.
//   * Grouping + OS awareness. A set whose labels are all operating systems is
//     emitted as `OSTab` (shared `os` group + a user-agent default). Every
//     other multi-tab set is emitted as `Tabs` with a `groupId` derived from
//     its labels and `persist`, so repeated sets (CLI/UI, Docker/Kubernetes,
//     ...) stay in sync and survive reloads.

import { isHtml, type MdastNode } from "./mdast";

const TABS_OPEN = /^<div\s+class="tabs"\s*>/i;

// Net `<div>` nesting change contributed by a raw html node's text.
function divDelta(value: string): number {
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

// Canonical operating-system names, keyed by their lowercased spellings as they
// appear in the corpus.
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

function canonicalOS(token: string): string | null {
	return OS_CANONICAL[token.trim().toLowerCase()] ?? null;
}

// If a label is only OS names joined by `/`, `,`, `&`, or `and`, return the
// canonical OS names (deduped, in order). Otherwise return null, so non-OS
// labels such as "Debian, Ubuntu" are left untouched.
function splitOSLabel(label: string): string[] | null {
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

// Build the `items={[...]}` attribute. Fumadocs needs a real array expression
// (it renders the trigger list from `items`), so emit an mdast expression
// attribute backed by an estree ArrayExpression of string literals.
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

// A boolean JSX attribute (`persist`). Backed by an estree `true` literal so it
// survives the plain-Markdown compile path the same way `items` does.
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

// Stable identifier so tab sets that share a label set stay in sync. Sorted so
// order does not matter, lowercased, and reduced to a slug.
function groupIdFor(labels: string[]): string {
	return [...labels]
		.map((label) => label.toLowerCase().trim())
		.sort()
		.join("|")
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
}

type TabDef = { label: string; content: MdastNode[] };

// Expand any bundled OS label into one tab per OS, duplicating the content. The
// first OS reuses the original nodes; the rest get structural clones so later
// passes cannot mutate shared nodes.
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
			out.push(tab);
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

	// Convert nested tab blocks before splitting, so an inner `<div class="tabs">`
	// becomes a single element (kept as tab content) rather than leaking its
	// headings into the outer split.
	const inner = transformList(children.slice(openIndex + 1, closeIndex));

	// The first heading sets the tab-delimiter depth; each heading at that depth
	// starts a new tab. Anything before the first heading is kept outside.
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

// Convert every `<div class="tabs">` block in a sibling list, recursing into
// child lists first so tabs nested inside other nodes are handled too.
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
