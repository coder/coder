/*
 * Adapted from rehype-github-alert.
 *
 * The MIT License
 *
 * Copyright (c) Titus Wormer <tituswormer@gmail.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 *
 * @see {@link https://github.com/rehypejs/rehype-github/tree/main/packages/alert}
 */

import type { Root } from "hast";
import { whitespace } from "hast-util-whitespace";
import { visit } from "unist-util-visit";

interface Icon {
	d: string;
	name: string;
}

const icons: Map<string, Icon> = new Map();

const TABS_CLASSNAME = "tabs";

icons.set("caution", {
	d: "M4.47.22A.749.749 0 0 1 5 0h6c.199 0 .389.079.53.22l4.25 4.25c.141.14.22.331.22.53v6a.749.749 0 0 1-.22.53l-4.25 4.25A.749.749 0 0 1 11 16H5a.749.749 0 0 1-.53-.22L.22 11.53A.749.749 0 0 1 0 11V5c0-.199.079-.389.22-.53Zm.84 1.28L1.5 5.31v5.38l3.81 3.81h5.38l3.81-3.81V5.31L10.69 1.5ZM8 4a.75.75 0 0 1 .75.75v3.5a.75.75 0 0 1-1.5 0v-3.5A.75.75 0 0 1 8 4Zm0 8a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z",
	name: "octicon-stop",
});
icons.set("important", {
	d: "M0 1.75C0 .784.784 0 1.75 0h12.5C15.216 0 16 .784 16 1.75v9.5A1.75 1.75 0 0 1 14.25 13H8.06l-2.573 2.573A1.458 1.458 0 0 1 3 14.543V13H1.75A1.75 1.75 0 0 1 0 11.25Zm1.75-.25a.25.25 0 0 0-.25.25v9.5c0 .138.112.25.25.25h2a.75.75 0 0 1 .75.75v2.19l2.72-2.72a.749.749 0 0 1 .53-.22h6.5a.25.25 0 0 0 .25-.25v-9.5a.25.25 0 0 0-.25-.25Zm7 2.25v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 9a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z",
	name: "octicon-report",
});
icons.set("note", {
	d: "M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13ZM6.5 7.75A.75.75 0 0 1 7.25 7h1a.75.75 0 0 1 .75.75v2.75h.25a.75.75 0 0 1 0 1.5h-2a.75.75 0 0 1 0-1.5h.25v-2h-.25a.75.75 0 0 1-.75-.75ZM8 6a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z",
	name: "octicon-info",
});
icons.set("tip", {
	d: "M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.537.896.621 1.49a.75.75 0 0 1-1.484.211c-.04-.282-.163-.547-.37-.847a8.456 8.456 0 0 0-.542-.68c-.084-.1-.173-.205-.268-.32C3.201 7.75 2.5 6.766 2.5 5.25 2.5 2.31 4.863 0 8 0s5.5 2.31 5.5 5.25c0 1.516-.701 2.5-1.328 3.259-.095.115-.184.22-.268.319-.207.245-.383.453-.541.681-.208.3-.33.565-.37.847a.751.751 0 0 1-1.485-.212c.084-.593.337-1.078.621-1.489.203-.292.45-.584.673-.848.075-.088.147-.173.213-.253.561-.679.985-1.32.985-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 12h4.5a.75.75 0 0 1 0 1.5h-4.5a.75.75 0 0 1 0-1.5ZM6 15.25a.75.75 0 0 1 .75-.75h2.5a.75.75 0 0 1 0 1.5h-2.5a.75.75 0 0 1-.75-.75Z",
	name: "octicon-light-bulb",
});
icons.set("warning", {
	d: "M6.457 1.047c.659-1.234 2.427-1.234 3.086 0l6.082 11.378A1.75 1.75 0 0 1 14.082 15H1.918a1.75 1.75 0 0 1-1.543-2.575Zm1.763.707a.25.25 0 0 0-.44 0L1.698 13.132a.25.25 0 0 0 .22.368h12.164a.25.25 0 0 0 .22-.368Zm.53 3.996v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z",
	name: "octicon-alert",
});

/**
 * Plugin to enhance alerts.
 *
 * @returns
 *   Transform.
 */
export default function coderGithubAlert() {
	return (tree: Root) => {
		visit(tree, "element", (node, index, parent) => {
			if (
				node.tagName !== "blockquote" ||
				typeof index !== "number" ||
				!parent
			) {
				return;
			}

			const parentClassName =
				parent.type === "element" ? parent.properties.className : "";
			const isInTabs = Array.isArray(parentClassName)
				? parentClassName.includes(TABS_CLASSNAME)
				: parentClassName === TABS_CLASSNAME;
			const isInAllowedParent =
				parent.type === "root" ||
				parent.tagName === "li" ||
				parent.tagName === "details" ||
				isInTabs;

			// Must be a `blockquote` element that is a direct child of the root,
			// a list item, a details element, or a tabs container.
			if (!isInAllowedParent) {
				return;
			}

			// Must start with a `<p>`.
			let headIndex = 0;

			while (
				headIndex < node.children.length &&
				whitespace(node.children[headIndex])
			) {
				headIndex++;
			}

			const head = node.children[headIndex];

			// Must start with a `p`.
			if (!head || head.type !== "element" || head.tagName !== "p") return;

			// Must start with `[!`.
			const text = head.children[0];
			if (!text || text.type !== "text" || !text.value.startsWith("[!")) return;

			// Must have `]`.
			const end = text.value.indexOf("]");
			if (end === -1) return;

			// Must be a known name between
			const name = text.value.slice(2, end).toLowerCase();
			const icon = icons.get(name);
			if (!icon) return;

			if (end + 1 === text.value.length) {
				// Has to be a `<br>`.
				const next = head.children[1];

				if (next) {
					if (next.type !== "element" || next.tagName !== "br") return;
					// No next sibling? Exit too.
					if (!head.children[2]) return;
					// Drop the entire `[!note]` text *and* the `<br>`.
					head.children = head.children.slice(2);
					// Drop the `\n` that typically (in markdown) follows `<br>`.
					const node = head.children[0];
					if (node && node.type === "text" && node.value.charAt(0) === "\n") {
						node.value = node.value.slice(1);
					}
				} else {
					let skipped = false;

					// Skip past whitespace.
					while (
						headIndex + 1 < node.children.length &&
						whitespace(node.children[headIndex + 1])
					) {
						skipped = true;
						headIndex++;
					}

					// Exit if there’s no following element.
					if (
						headIndex + 1 === node.children.length ||
						node.children[headIndex + 1].type !== "element"
					) {
						return;
					}

					// Without whitespace,
					// we still want to skip the paragraph to drop the `[!note]`.
					if (!skipped) headIndex++;
				}
			} else if (
				text.value.charAt(end + 1) === "\n" &&
				(end + 2 === text.value.length ||
					!whitespace(text.value.slice(end + 2)))
			) {
				// Drop the `[!note]` from the `text`.
				text.value = text.value.slice(end + 2);
			} else {
				return;
			}

			const displayName = name.charAt(0).toUpperCase() + name.slice(1);

			parent.children[index] = {
				type: "element",
				tagName: "div",
				properties: { className: ["markdown-alert", `markdown-alert-${name}`] },
				children: [
					{
						type: "element",
						tagName: "p",
						properties: { className: ["markdown-alert-title"] },
						children: [
							{
								type: "element",
								tagName: "svg",
								properties: {
									className: ["octicon", icon.name],
									viewBox: "0 0 16 16",
									version: "1.1",
									width: "16",
									height: "16",
									ariaHidden: "true",
								},
								children: [
									{
										type: "element",
										tagName: "path",
										properties: { d: icon.d },
										children: [],
									},
								],
							},
							{ type: "text", value: displayName },
						],
					},
					...node.children.slice(headIndex),
				],
			};
		});
	};
}
