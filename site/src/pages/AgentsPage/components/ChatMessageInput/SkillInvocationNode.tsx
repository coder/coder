import { cn } from "cn";
import {
	$createLineBreakNode,
	$createParagraphNode,
	$createTextNode,
	$getNodeByKey,
	$getRoot,
	$isParagraphNode,
	$isTextNode,
	createEditor,
	DecoratorNode,
	type LexicalEditor,
	type NodeKey,
	type RangeSelection,
	type SerializedLexicalNode,
	type Spread,
} from "lexical";
import type { ChatMessagePart } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import type { SkillSource } from "../../utils/skillAliases";
import {
	$createFileReferenceNode,
	FileReferenceNode,
	useFileReferenceSpacing,
} from "./FileReferenceNode";
import { EditableSkillChip } from "./SkillChip";

type SerializedSkillInvocationNode = Spread<
	{ source: SkillSource; name: string },
	SerializedLexicalNode
>;

export const isSerializedSkillInvocationNode = (
	node: SerializedLexicalNode,
): node is SerializedSkillInvocationNode => node.type === "skill-invocation";

/** A skill chip that sends as a skill-invocation part. */
export class SkillInvocationNode extends DecoratorNode<React.ReactNode> {
	__source: SkillSource;
	__name: string;

	static getType(): string {
		return "skill-invocation";
	}

	static clone(node: SkillInvocationNode): SkillInvocationNode {
		return new SkillInvocationNode(node.__source, node.__name, node.__key);
	}

	constructor(source: SkillSource, name: string, key?: NodeKey) {
		super(key);
		this.__source = source;
		this.__name = name;
	}

	createDOM(): HTMLElement {
		const span = document.createElement("span");
		span.className = "inline select-none";
		return span;
	}

	updateDOM(): boolean {
		return false;
	}

	exportJSON(): SerializedSkillInvocationNode {
		return {
			type: "skill-invocation",
			version: 1,
			source: this.__source,
			name: this.__name,
		};
	}

	static importJSON(json: SerializedSkillInvocationNode): SkillInvocationNode {
		return new SkillInvocationNode(json.source, json.name);
	}

	getTextContent(): string {
		return "";
	}

	isInline(): boolean {
		return true;
	}

	// A keyboard-selected decorator becomes a NodeSelection, which clears
	// the DOM caret while the editor keeps focus, so the next keystroke
	// lands at the start of the editor. Arrow keys step over the chip.
	isKeyboardSelectable(): boolean {
		return false;
	}

	decorate(editor: LexicalEditor): React.ReactNode {
		return (
			<SkillChipWrapper
				editor={editor}
				nodeKey={this.__key}
				source={this.__source}
				name={this.__name}
			/>
		);
	}
}

const SkillChipWrapper: React.FC<{
	editor: LexicalEditor;
	nodeKey: NodeKey;
	source: SkillSource;
	name: string;
}> = ({ editor, nodeKey, source, name }) => {
	const spacing = useFileReferenceSpacing(editor, nodeKey);
	const handleRemove = () => {
		editor.update(() => {
			$getNodeByKey(nodeKey)?.remove();
		});
		editor.getRootElement()?.focus({ preventScroll: true });
	};
	return (
		<EditableSkillChip
			source={source}
			name={name}
			onRemove={handleRemove}
			className={cn(spacing.before && "ml-1", spacing.after && "mr-1")}
		/>
	);
};

/** chatd rejects a message that invokes more skills than this. */
export const MAX_SKILL_INVOCATIONS_PER_MESSAGE = 8;

/** Reports chips rejected by the limit, for direct insertion and paste alike. */
export const warnSkillLimit = () => {
	toast.error(
		`A message can include up to ${MAX_SKILL_INVOCATIONS_PER_MESSAGE} skills.`,
	);
};

export const $getSkillInvocationNodes = (): SkillInvocationNode[] =>
	$getRoot()
		.getChildren()
		.flatMap((paragraph) =>
			$isParagraphNode(paragraph)
				? paragraph
						.getChildren()
						.filter((node) => node instanceof SkillInvocationNode)
				: [],
		);

export const $hasSkillInvocationNode = (source: SkillSource, name: string) =>
	$getSkillInvocationNodes().some(
		(node) => node.__source === source && node.__name === name,
	);

/**
 * Replaces the selection with a skill chip followed by a space. A skill
 * already in the editor, or one past the limit, only removes the selection.
 */
export function $insertSkillInvocation(
	selection: RangeSelection,
	source: SkillSource,
	name: string,
) {
	if ($hasSkillInvocationNode(source, name)) {
		selection.removeText();
		return;
	}
	if ($getSkillInvocationNodes().length >= MAX_SKILL_INVOCATIONS_PER_MESSAGE) {
		selection.removeText();
		warnSkillLimit();
		return;
	}
	const chip = new SkillInvocationNode(source, name);
	selection.insertNodes([chip]);
	const next = chip.getNextSibling();
	if ($isTextNode(next) && /^\s/.test(next.getTextContent())) {
		next.select(1, 1);
		return;
	}
	chip.selectNext().insertText(" ");
}

/**
 * Serializes a sent message's text, skill chips and file-reference chips as
 * a composer draft.
 */
export const serializeMessageDraft = (
	content: readonly ChatMessagePart[],
): string => {
	const editor = createEditor({
		nodes: [SkillInvocationNode, FileReferenceNode],
		onError: (error) => {
			throw error;
		},
	});
	editor.update(
		() => {
			const paragraph = $createParagraphNode();
			for (const part of content) {
				if (part.type === "skill-invocation") {
					paragraph.append(
						new SkillInvocationNode(part.skill_source, part.skill_name),
					);
				} else if (part.type === "file-reference") {
					paragraph.append(
						$createFileReferenceNode(
							part.file_name,
							part.start_line,
							part.end_line,
							part.content,
						),
					);
				} else if (part.type === "text") {
					for (const [index, line] of part.text.split("\n").entries()) {
						if (index > 0) {
							paragraph.append($createLineBreakNode());
						}
						if (line) {
							paragraph.append($createTextNode(line));
						}
					}
				}
			}
			$getRoot().append(paragraph);
		},
		{ discrete: true },
	);
	return JSON.stringify(editor.getEditorState().toJSON());
};
