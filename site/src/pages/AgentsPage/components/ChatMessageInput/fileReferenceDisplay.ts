import type { LexicalEditor, LexicalNode, NodeKey } from "lexical";
import { $getNodeByKey } from "lexical";
import { getPathBasename } from "../../utils/path";

export type InlinePart =
	| { readonly type: "file-reference" }
	| { readonly type: "text"; readonly text: string };

const isFileReferenceNode = (node: LexicalNode) => {
	return node.getType() === "file-reference";
};

export const getFileReferenceDisplay = ({
	fileName,
	startLine,
	endLine,
}: {
	fileName: string;
	startLine: number;
	endLine: number;
}) => {
	const shortFile = getPathBasename(fileName);
	const lineRange =
		startLine === endLine ? `L${startLine}` : `L${startLine}-L${endLine}`;
	const title = `${fileName}:${lineRange}`;

	return { shortFile, lineRange, title };
};

export const hasInlineContentBefore = (
	parts: readonly InlinePart[],
	index: number,
) => {
	for (let i = index - 1; i >= 0; i--) {
		const part = parts[i];
		if (part.type === "file-reference") {
			return true;
		}
		if (part.text.length > 0) {
			return !/\s$/.test(part.text);
		}
	}
	return false;
};

export const hasInlineContentAfter = (
	parts: readonly InlinePart[],
	index: number,
) => {
	for (let i = index + 1; i < parts.length; i++) {
		const part = parts[i];
		if (part.type === "file-reference") {
			return true;
		}
		if (part.text.length > 0) {
			return !/^\s/.test(part.text);
		}
	}
	return false;
};

export const getFileReferenceSiblingSpacing = (
	editor: LexicalEditor,
	nodeKey: NodeKey,
) => {
	const parts: InlinePart[] = [];
	let referenceIndex = -1;

	editor.getEditorState().read(() => {
		const node = $getNodeByKey(nodeKey);
		if (!node) {
			return;
		}

		let sibling = node.getPreviousSibling();
		const previousParts: InlinePart[] = [];
		while (sibling) {
			if (isFileReferenceNode(sibling)) {
				previousParts.unshift({ type: "file-reference" });
			} else {
				previousParts.unshift({ type: "text", text: sibling.getTextContent() });
			}
			sibling = sibling.getPreviousSibling();
		}

		parts.push(...previousParts);
		referenceIndex = parts.length;
		parts.push({ type: "file-reference" });

		sibling = node.getNextSibling();
		while (sibling) {
			if (isFileReferenceNode(sibling)) {
				parts.push({ type: "file-reference" });
			} else {
				parts.push({ type: "text", text: sibling.getTextContent() });
			}
			sibling = sibling.getNextSibling();
		}
	});

	return {
		after: referenceIndex >= 0 && hasInlineContentAfter(parts, referenceIndex),
		before:
			referenceIndex >= 0 && hasInlineContentBefore(parts, referenceIndex),
	};
};
