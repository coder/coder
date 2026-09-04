import type { ElementContent, Root } from "hast";
import { createContext, type ReactNode, useContext } from "react";
import type { Components, UrlTransform } from "streamdown";
import { cn } from "#/utils/cn";
import { Response } from "../ChatElements/Response";
import { FileReferenceChip } from "../ChatMessageInput/FileReferenceChip";
import {
	getFileReferenceDisplay,
	hasInlineContentAfter,
	hasInlineContentBefore,
	type InlinePart,
} from "../ChatMessageInput/fileReferenceDisplay";
import type { UserInlineRenderBlock } from "./messageHelpers";

// Streamdown caches processors by plugin name and serialized options, not closure identity.
const rehypeFileReferences = ({
	prefix,
	labels,
}: {
	prefix: string;
	labels: Record<string, string>;
}) => {
	const referenceMarkers = Object.keys(labels);
	const markers = new RegExp(`(${prefix}\\d+\uE001)`, "g");
	const literalText = (value: string) =>
		value.replace(markers, (marker) => labels[marker] ?? marker);
	return (tree: Root) => {
		const visit = (node: Root | ElementContent, literal = false): void => {
			if (!("children" in node)) return;
			if (node.type === "element") {
				literal ||= node.tagName === "code" || node.tagName === "pre";
				for (const [key, value] of Object.entries(node.properties)) {
					if (typeof value !== "string") continue;
					if (key === "href" || key === "src") {
						if (
							referenceMarkers.some(
								(marker) =>
									value.includes(marker) ||
									value.includes(encodeURIComponent(marker)),
							)
						) {
							delete node.properties[key];
						}
					} else {
						node.properties[key] = literalText(value);
					}
				}
			}
			node.children = node.children.flatMap((child): ElementContent[] => {
				if (child.type === "text") {
					if (!child.value.includes(prefix)) return [child];
					if (literal) return [{ ...child, value: literalText(child.value) }];
					return child.value
						.split(markers)
						.filter(Boolean)
						.map((value) =>
							Object.hasOwn(labels, value)
								? {
										type: "element",
										tagName: "span",
										properties: {},
										children: [{ type: "text", value }],
									}
								: { type: "text", value },
						);
				}
				if (child.type === "doctype") return [];
				visit(child, literal);
				return [child];
			});
		};
		visit(tree);
	};
};

const FileReferencesContext = createContext<ReadonlyMap<string, ReactNode>>(
	new Map(),
);

const components: Components = {
	span: ({ node, children, ...props }) => {
		const references = useContext(FileReferencesContext);
		const child = node?.children[0];
		return (
			(node?.children.length === 1 &&
				child?.type === "text" &&
				references.get(child.value)) || <span {...props}>{children}</span>
		);
	},
};

export const UserMessageMarkdown = ({
	blocks,
	markdown,
	urlTransform,
}: {
	blocks: readonly UserInlineRenderBlock[];
	markdown: string;
	urlTransform?: UrlTransform;
}) => {
	const parts: InlinePart[] = blocks.map((block) =>
		block.type === "response"
			? { type: "text", text: block.text }
			: { type: "file-reference" },
	);
	const text = blocks
		.filter((block) => block.type === "response")
		.map((block) => block.text)
		.join("");
	let prefix = "\uE000";
	while (text.includes(prefix)) prefix += "\uE000";
	const references = new Map<string, ReactNode>();
	const labels: Record<string, string> = {};
	// Opaque markers keep Markdown containers intact without parsing file metadata.
	const source = blocks.length
		? blocks
				.map((block, index) => {
					if (block.type === "response") return block.text;
					const marker = `${prefix}${index}\uE001`;
					const { title } = getFileReferenceDisplay({
						fileName: block.file_name,
						startLine: block.start_line,
						endLine: block.end_line,
					});
					labels[marker] = title;
					references.set(
						marker,
						<FileReferenceChip
							fileName={block.file_name}
							startLine={block.start_line}
							endLine={block.end_line}
							className={cn(
								hasInlineContentBefore(parts, index) && "ml-1",
								hasInlineContentAfter(parts, index) && "mr-1",
							)}
						/>,
					);
					return marker;
				})
				.join("")
		: markdown;
	return (
		<FileReferencesContext value={references}>
			<Response
				className="min-w-0 flex-1 [overflow-wrap:anywhere]"
				urlTransform={urlTransform}
				components={references.size ? components : undefined}
				rehypePlugins={
					references.size
						? [[rehypeFileReferences, { prefix, labels }]]
						: undefined
				}
			>
				{source}
			</Response>
		</FileReferencesContext>
	);
};
