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
import { cva, type VariantProps } from "class-variance-authority";
import { XIcon } from "lucide-react";
import {
	type CSSProperties,
	type FC,
	memo,
	type ReactNode,
	useEffect,
	useState,
} from "react";
import { FileIcon } from "#/components/FileIcon/FileIcon";
import { cn } from "#/utils/cn";

type SerializedFileReferenceNode = Spread<
	{
		fileName: string;
		startLine: number;
		endLine: number;
		content: string;
	},
	SerializedLexicalNode
>;

const fileReferenceChipVariants = cva(
	"inline-flex min-h-5 max-w-[300px] select-none items-center gap-1 rounded-md border border-border-default bg-surface-primary py-0 pl-0.5 pr-1.5 align-middle font-sans text-[13px] font-normal leading-none text-inherit shadow-sm transition-colors",
	{
		variants: {
			interactive: {
				true: "cursor-pointer hover:border-border-secondary hover:bg-surface-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
				false: "cursor-default",
			},
			selected: {
				true: "border-content-link bg-content-link/10 text-content-primary ring-1 ring-content-link/40",
				false: "",
			},
		},
		defaultVariants: {
			interactive: false,
			selected: false,
		},
	},
);

const fileReferenceTriggerVariants = cva(
	"inline-flex min-w-0 items-center gap-1 border-0 bg-transparent p-0 font-sans text-[13px] font-normal leading-none text-inherit",
	{
		variants: {
			interactive: {
				true: "cursor-pointer focus-visible:outline-none",
				false: "cursor-default",
			},
		},
		defaultVariants: {
			interactive: false,
		},
	},
);

const fileReferenceIconStyle: CSSProperties = {
	fontSize: 16,
	height: "1rem",
	minWidth: "1rem",
};

type FileReferenceChipContentProps = {
	fileName: string;
	lineLabel: string;
};

const FileReferenceChipContent: FC<FileReferenceChipContentProps> = ({
	fileName,
	lineLabel,
}) => {
	return (
		<>
			<FileIcon
				fileName={fileName}
				className="shrink-0"
				style={fileReferenceIconStyle}
			/>
			<span
				data-slot="file-reference-chip-label"
				className="inline-flex min-w-0 items-center gap-0.5"
			>
				<span dir="rtl" className="min-w-0 truncate">
					{fileName}
				</span>
				<span className="shrink-0">·</span>
				<span className="shrink-0 tabular-nums">{lineLabel}</span>
			</span>
		</>
	);
};

type FileReferenceChipBaseProps = {
	fileName: string;
	startLine: number;
	endLine: number;
	className?: string;
};

type FileReferenceChipProps = FileReferenceChipBaseProps &
	VariantProps<typeof fileReferenceChipVariants>;

const getFileReferenceDisplay = ({
	fileName,
	startLine,
	endLine,
}: Pick<FileReferenceChipBaseProps, "fileName" | "startLine" | "endLine">) => {
	const shortFile = fileName.split("/").pop() || fileName;
	const lineLabel =
		startLine === endLine ? `${startLine}` : `${startLine}-${endLine}`;
	const title = `${fileName}:L${lineLabel}`;

	return { shortFile, lineLabel, title };
};

export function FileReferenceChip({
	fileName,
	startLine,
	endLine,
	selected,
	className,
}: FileReferenceChipProps) {
	const { shortFile, lineLabel, title } = getFileReferenceDisplay({
		fileName,
		startLine,
		endLine,
	});

	return (
		<span
			data-slot="file-reference-chip"
			className={cn(fileReferenceChipVariants({ selected }), className)}
			contentEditable={false}
			title={title}
		>
			<span
				data-slot="file-reference-chip-trigger"
				className={fileReferenceTriggerVariants()}
			>
				<FileReferenceChipContent fileName={shortFile} lineLabel={lineLabel} />
			</span>
		</span>
	);
}

export function EditableFileReferenceChip({
	fileName,
	startLine,
	endLine,
	selected,
	onRemove,
	onOpen,
	className,
}: FileReferenceChipBaseProps & {
	selected?: boolean;
	onRemove: () => void;
	onOpen: () => void;
}) {
	const { shortFile, lineLabel, title } = getFileReferenceDisplay({
		fileName,
		startLine,
		endLine,
	});

	return (
		<span
			data-slot="file-reference-chip"
			className={cn(
				fileReferenceChipVariants({ interactive: true, selected }),
				"border-border-secondary",
				className,
			)}
			contentEditable={false}
			title={title}
		>
			<button
				data-slot="file-reference-chip-trigger"
				type="button"
				className={fileReferenceTriggerVariants({ interactive: true })}
				onClick={onOpen}
			>
				<FileReferenceChipContent fileName={shortFile} lineLabel={lineLabel} />
			</button>
			<button
				data-slot="file-reference-chip-remove"
				type="button"
				className="ml-0.5 inline-flex size-3.5 shrink-0 cursor-pointer items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary transition-colors hover:bg-surface-quaternary hover:text-content-primary"
				onClick={(e) => {
					e.preventDefault();
					e.stopPropagation();
					onRemove();
				}}
				aria-label="Remove reference"
				tabIndex={-1}
			>
				<XIcon className="size-2.5" />
			</button>
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

	createDOM(config: EditorConfig): HTMLElement {
		const span = document.createElement("span");
		span.className = config.theme.inlineDecorator ?? "";
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

const hasContentBeforeReference = (editor: LexicalEditor, nodeKey: NodeKey) => {
	let hasContentBefore = false;
	editor.getEditorState().read(() => {
		const node = $getNodeByKey(nodeKey);
		let sibling = node?.getPreviousSibling();

		while (sibling) {
			if (sibling instanceof FileReferenceNode) {
				hasContentBefore = true;
				return;
			}

			const text = sibling.getTextContent();
			if (text.length > 0) {
				hasContentBefore = !/\s$/.test(text);
				return;
			}

			sibling = sibling.getPreviousSibling();
		}
	});

	return hasContentBefore;
};

const hasContentAfterReference = (editor: LexicalEditor, nodeKey: NodeKey) => {
	let hasContentAfter = false;
	editor.getEditorState().read(() => {
		const node = $getNodeByKey(nodeKey);
		let sibling = node?.getNextSibling();

		while (sibling) {
			if (sibling instanceof FileReferenceNode) {
				hasContentAfter = true;
				return;
			}

			const text = sibling.getTextContent();
			if (text.length > 0) {
				hasContentAfter = !/^\s/.test(text);
				return;
			}

			sibling = sibling.getNextSibling();
		}
	});

	return hasContentAfter;
};

const FileReferenceChipWrapper: FC<{
	editor: LexicalEditor;
	nodeKey: NodeKey;
	fileName: string;
	startLine: number;
	endLine: number;
}> = memo(({ editor, nodeKey, fileName, startLine, endLine }) => {
	const [isSelected] = useLexicalNodeSelection(nodeKey);
	const [hasContentBefore, setHasContentBefore] = useState(() =>
		hasContentBeforeReference(editor, nodeKey),
	);
	const [hasContentAfter, setHasContentAfter] = useState(() =>
		hasContentAfterReference(editor, nodeKey),
	);

	useEffect(() => {
		return editor.registerUpdateListener(() => {
			setHasContentBefore(hasContentBeforeReference(editor, nodeKey));
			setHasContentAfter(hasContentAfterReference(editor, nodeKey));
		});
	}, [editor, nodeKey]);

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
			className={cn(hasContentBefore && "ml-1", hasContentAfter && "mr-1")}
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
