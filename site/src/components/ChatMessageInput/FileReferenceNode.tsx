import { useLexicalNodeSelection } from "@lexical/react/useLexicalNodeSelection";
import { FileIcon } from "components/FileIcon/FileIcon";
import {
	$getNodeByKey,
	DecoratorNode,
	type EditorConfig,
	type LexicalEditor,
	type NodeKey,
	type SerializedLexicalNode,
	type Spread,
} from "lexical";
import { XIcon } from "lucide-react";
import { type FC, memo, type ReactNode } from "react";
import { cn } from "utils/cn";

type SerializedFileReferenceNode = Spread<
	{
		fileName: string;
		startLine: number;
		endLine: number;
		content: string;
	},
	SerializedLexicalNode
>;

export function FileReferenceChip({
	fileName,
	startLine,
	endLine,
	isSelected,
	onRemove,
	onClick,
}: {
	fileName: string;
	startLine: number;
	endLine: number;
	isSelected?: boolean;
	onRemove?: () => void;
	onClick?: () => void;
}) {
	const shortFile = fileName.split("/").pop() || fileName;
	const lineLabel =
		startLine === endLine ? `L${startLine}` : `L${startLine}–${endLine}`;

	return (
		<span
			className={cn(
				"inline-flex h-6 max-w-[300px] cursor-pointer select-none items-center gap-1.5 rounded-md border border-border-default bg-surface-secondary px-1.5 align-middle text-xs text-content-primary shadow-sm transition-colors",
				isSelected &&
					"border-content-link bg-content-link/10 ring-1 ring-content-link/40",
			)}
			contentEditable={false}
			title={`${fileName}:${lineLabel}`}
			onClick={onClick}
			onKeyDown={(e) => {
				if (e.key === "Enter" || e.key === " ") {
					e.preventDefault();
					onClick?.();
				}
			}}
			role="button"
			tabIndex={0}
		>
			<FileIcon fileName={shortFile} className="shrink-0" />
			<span className="shrink-0 text-content-secondary">
				{shortFile}
				<span className="text-content-link">:{lineLabel}</span>
			</span>
			{onRemove && (
				<button
					type="button"
					className="ml-auto inline-flex size-4 shrink-0 items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary transition-colors hover:text-content-primary cursor-pointer"
					onClick={(e) => {
						e.preventDefault();
						e.stopPropagation();
						onRemove();
					}}
					aria-label="Remove reference"
					tabIndex={-1}
				>
					<XIcon className="size-2" />
				</button>
			)}
		</span>
	);
}

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

const FileReferenceChipWrapper: FC<{
	editor: LexicalEditor;
	nodeKey: NodeKey;
	fileName: string;
	startLine: number;
	endLine: number;
}> = memo(({ editor, nodeKey, fileName, startLine, endLine }) => {
	const [isSelected] = useLexicalNodeSelection(nodeKey);

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
		<FileReferenceChip
			fileName={fileName}
			startLine={startLine}
			endLine={endLine}
			isSelected={isSelected}
			onRemove={handleRemove}
			onClick={handleClick}
		/>
	);
});
FileReferenceChipWrapper.displayName = "FileReferenceChipWrapper";

export function $createFileReferenceNode(
	fileName: string,
	startLine: number,
	endLine: number,
	content: string,
): FileReferenceNode {
	return new FileReferenceNode(fileName, startLine, endLine, content);
}
