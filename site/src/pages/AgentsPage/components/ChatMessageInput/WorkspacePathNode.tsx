import { useLexicalNodeSelection } from "@lexical/react/useLexicalNodeSelection";
import { cn } from "cn";
import {
	$getNodeByKey,
	DecoratorNode,
	type LexicalEditor,
	type NodeKey,
	type SerializedLexicalNode,
	type Spread,
} from "lexical";
import { WorkspacePathChip } from "./FileReferenceChip";
import { useFileReferenceSpacing } from "./FileReferenceNode";

type SerializedWorkspacePathNode = Spread<
	{
		path: string;
		isDirectory: boolean;
	},
	SerializedLexicalNode
>;

/**
 * A workspace file or folder picked from the "@" menu. Its text content is
 * the absolute path in backticks, with a trailing "/" for a folder, so the
 * sent message, copied text, and drafts carry a plain path that the model
 * can read and the transcript renders as a file link.
 */
export class WorkspacePathNode extends DecoratorNode<React.ReactNode> {
	__path: string;
	__isDirectory: boolean;

	static getType(): string {
		return "workspace-path";
	}

	static clone(node: WorkspacePathNode): WorkspacePathNode {
		return new WorkspacePathNode(node.__path, node.__isDirectory, node.__key);
	}

	constructor(path: string, isDirectory: boolean, key?: NodeKey) {
		super(key);
		this.__path = path;
		this.__isDirectory = isDirectory;
	}

	createDOM(): HTMLElement {
		const span = document.createElement("span");
		span.style.display = "inline";
		span.style.userSelect = "none";
		return span;
	}

	updateDOM(): boolean {
		return false;
	}

	exportJSON(): SerializedWorkspacePathNode {
		return {
			type: "workspace-path",
			version: 1,
			path: this.__path,
			isDirectory: this.__isDirectory,
		};
	}

	static importJSON(json: SerializedWorkspacePathNode): WorkspacePathNode {
		return new WorkspacePathNode(json.path, json.isDirectory);
	}

	getTextContent(): string {
		return `\`${this.__path}${this.__isDirectory ? "/" : ""}\``;
	}

	isInline(): boolean {
		return true;
	}

	decorate(editor: LexicalEditor): React.ReactNode {
		return (
			<WorkspacePathChipWrapper
				editor={editor}
				nodeKey={this.__key}
				path={this.__path}
				isDirectory={this.__isDirectory}
			/>
		);
	}
}

const WorkspacePathChipWrapper: React.FC<{
	editor: LexicalEditor;
	nodeKey: NodeKey;
	path: string;
	isDirectory: boolean;
}> = ({ editor, nodeKey, path, isDirectory }) => {
	const [isSelected] = useLexicalNodeSelection(nodeKey);
	const spacing = useFileReferenceSpacing(editor, nodeKey);

	const handleRemove = () => {
		editor.update(() => {
			$getNodeByKey(nodeKey)?.remove();
		});
	};

	return (
		<WorkspacePathChip
			path={path}
			isDirectory={isDirectory}
			selected={isSelected}
			onRemove={handleRemove}
			className={cn(spacing.before && "ml-1", spacing.after && "mr-1")}
		/>
	);
};

export function $createWorkspacePathNode(
	path: string,
	isDirectory: boolean,
): WorkspacePathNode {
	return new WorkspacePathNode(path, isDirectory);
}
