import { useLexicalNodeSelection } from "@lexical/react/useLexicalNodeSelection";
import {
	$getNodeByKey,
	DecoratorNode,
	type EditorConfig,
	type LexicalEditor,
	type NodeKey,
	type SerializedLexicalNode,
	type Spread,
} from "lexical";
import { type FC, type ReactNode, useSyncExternalStore } from "react";
import { cn } from "#/utils/cn";
import { EditableFileReferenceChip } from "./FileReferenceChip";
import { getFileReferenceSiblingSpacing } from "./fileReferenceDisplay";

type SerializedFileReferenceNode = Spread<
	{
		fileName: string;
		startLine: number;
		endLine: number;
		content: string;
	},
	SerializedLexicalNode
>;

export class FileReferenceNode extends DecoratorNode<ReactNode> {
	__fileName: string;
	__startLine: number;
	__endLine: number;
	__content: string;

	static getType(): string {
		return "file-reference";
	}

	static clone(node: FileReferenceNode): FileReferenceNode {
		return new FileReferenceNode(
			node.__fileName,
			node.__startLine,
			node.__endLine,
			node.__content,
			node.__key,
		);
	}

	constructor(
		fileName: string,
		startLine: number,
		endLine: number,
		content: string,
		key?: NodeKey,
	) {
		super(key);
		this.__fileName = fileName;
		this.__startLine = startLine;
		this.__endLine = endLine;
		this.__content = content;
	}

	createDOM(_config: EditorConfig): HTMLElement {
		const span = document.createElement("span");
		span.style.display = "inline";
		span.style.userSelect = "none";
		return span;
	}

	updateDOM(): boolean {
		return false;
	}

	exportJSON(): SerializedFileReferenceNode {
		return {
			type: "file-reference",
			version: 1,
			fileName: this.__fileName,
			startLine: this.__startLine,
			endLine: this.__endLine,
			content: this.__content,
		};
	}

	static importJSON(json: SerializedFileReferenceNode): FileReferenceNode {
		return new FileReferenceNode(
			json.fileName,
			json.startLine,
			json.endLine,
			json.content,
		);
	}

	getTextContent(): string {
		return "";
	}

	isInline(): boolean {
		return true;
	}

	decorate(_editor: LexicalEditor): ReactNode {
		return (
			<FileReferenceChipWrapper
				editor={_editor}
				nodeKey={this.__key}
				fileName={this.__fileName}
				startLine={this.__startLine}
				endLine={this.__endLine}
			/>
		);
	}
}

const SPACING_BEFORE = 1;
const SPACING_AFTER = 2;

const getFileReferenceSpacingSnapshot = (
	editor: LexicalEditor,
	nodeKey: NodeKey,
) => {
	const spacing = getFileReferenceSiblingSpacing(editor, nodeKey);
	return (
		(spacing.before ? SPACING_BEFORE : 0) | (spacing.after ? SPACING_AFTER : 0)
	);
};

const useFileReferenceSpacing = (editor: LexicalEditor, nodeKey: NodeKey) => {
	const spacingSnapshot = useSyncExternalStore(
		(notify) => editor.registerUpdateListener(notify),
		() => getFileReferenceSpacingSnapshot(editor, nodeKey),
	);

	return {
		after: (spacingSnapshot & SPACING_AFTER) !== 0,
		before: (spacingSnapshot & SPACING_BEFORE) !== 0,
	};
};

const FileReferenceChipWrapper: FC<{
	editor: LexicalEditor;
	nodeKey: NodeKey;
	fileName: string;
	startLine: number;
	endLine: number;
}> = ({ editor, nodeKey, fileName, startLine, endLine }) => {
	const [isSelected] = useLexicalNodeSelection(nodeKey);
	const spacing = useFileReferenceSpacing(editor, nodeKey);

	const handleRemove = () => {
		editor.update(() => {
			const node = $getNodeByKey(nodeKey);
			if (node instanceof FileReferenceNode) {
				node.remove();
			}
		});
	};

	const handleClick = () => {
		window.dispatchEvent(
			new CustomEvent("file-reference-click", {
				detail: { fileName, startLine, endLine },
			}),
		);
	};

	return (
		<EditableFileReferenceChip
			fileName={fileName}
			startLine={startLine}
			endLine={endLine}
			selected={isSelected}
			onRemove={handleRemove}
			onOpen={handleClick}
			className={cn(spacing.before && "ml-1", spacing.after && "mr-1")}
		/>
	);
};

export function $createFileReferenceNode(
	fileName: string,
	startLine: number,
	endLine: number,
	content: string,
): FileReferenceNode {
	return new FileReferenceNode(fileName, startLine, endLine, content);
}
