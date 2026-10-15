import { useLexicalNodeSelection } from "@lexical/react/useLexicalNodeSelection";
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
import type { SkillSource } from "../../utils/skillAliases";
import { useFileReferenceSpacing } from "./FileReferenceNode";
import { EditableSkillChip } from "./SkillChip";

type SerializedSkillInvocationNode = Spread<
	{ source: SkillSource; name: string },
	SerializedLexicalNode
>;

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
		span.style.display = "inline";
		span.style.userSelect = "none";
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
	const [isSelected] = useLexicalNodeSelection(nodeKey);
	const spacing = useFileReferenceSpacing(editor, nodeKey);
	const handleRemove = () => {
		editor.update(() => {
			$getNodeByKey(nodeKey)?.remove();
		});
	};
	return (
		<EditableSkillChip
			source={source}
			name={name}
			selected={isSelected}
			onRemove={handleRemove}
			className={cn(spacing.before && "ml-1", spacing.after && "mr-1")}
		/>
	);
};

const $hasSkillInvocationNode = (source: SkillSource, name: string) =>
	$getRoot()
		.getChildren()
		.some(
			(paragraph) =>
				$isParagraphNode(paragraph) &&
				paragraph
					.getChildren()
					.some(
						(node) =>
							node instanceof SkillInvocationNode &&
							node.__source === source &&
							node.__name === name,
					),
		);

/**
 * Replaces the selection with a skill chip followed by a space. A skill
 * already in the editor is not added again; the selection is only removed.
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
 * Serializes the text and skill-invocation parts of a sent message as a
 * composer draft, so editing the message keeps its skill chips in place.
 */
export const serializeMessageDraft = (
	content: readonly ChatMessagePart[],
): string => {
	const editor = createEditor({
		nodes: [SkillInvocationNode],
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
