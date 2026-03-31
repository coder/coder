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
import { XIcon, ZapIcon } from "lucide-react";
import { type FC, memo, type ReactNode } from "react";
import { cn } from "#/utils/cn";

export type SkillData = {
	skillName: string;
	skillDescription: string;
};

type SerializedSkillNode = Spread<
	{ skillName: string; skillDescription: string },
	SerializedLexicalNode
>;

export function SkillChip({
	skillName,
	skillDescription,
	isSelected,
	onRemove,
	className: extraClassName,
}: {
	skillName: string;
	skillDescription?: string;
	isSelected?: boolean;
	onRemove?: () => void;
	className?: string;
}) {
	const title = skillDescription
		? `${skillName}: ${skillDescription}`
		: skillName;

	return (
		<span
			className={cn(
				"inline-flex h-6 max-w-[300px] cursor-default select-none items-center gap-1.5 rounded-md border border-border-default bg-surface-primary px-1.5 align-middle text-xs text-content-primary shadow-sm transition-colors",
				isSelected &&
					"border-content-link bg-content-link/10 ring-1 ring-content-link/40",
				extraClassName,
			)}
			contentEditable={false}
			title={title}
		>
			<ZapIcon className="size-3 shrink-0" />
			<span className="min-w-0 truncate text-content-secondary">
				{skillName}
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
					aria-label="Remove skill"
					tabIndex={-1}
				>
					<XIcon className="size-2" />
				</button>
			)}
		</span>
	);
}

export class SkillNode extends DecoratorNode<ReactNode> {
	__skillName: string;
	__skillDescription: string;

	static getType(): string {
		return "skill-reference";
	}

	static clone(node: SkillNode): SkillNode {
		return new SkillNode(node.__skillName, node.__skillDescription, node.__key);
	}

	constructor(skillName: string, skillDescription: string, key?: NodeKey) {
		super(key);
		this.__skillName = skillName;
		this.__skillDescription = skillDescription;
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

	exportJSON(): SerializedSkillNode {
		return {
			type: "skill-reference",
			version: 1,
			skillName: this.__skillName,
			skillDescription: this.__skillDescription,
		};
	}

	static importJSON(json: SerializedSkillNode): SkillNode {
		return new SkillNode(json.skillName, json.skillDescription);
	}

	getTextContent(): string {
		return "";
	}

	isInline(): boolean {
		return true;
	}

	decorate(_editor: LexicalEditor): ReactNode {
		return (
			<SkillChipWrapper
				editor={_editor}
				nodeKey={this.__key}
				skillName={this.__skillName}
				skillDescription={this.__skillDescription}
			/>
		);
	}
}

const SkillChipWrapper: FC<{
	editor: LexicalEditor;
	nodeKey: NodeKey;
	skillName: string;
	skillDescription: string;
}> = memo(({ editor, nodeKey, skillName, skillDescription }) => {
	const [isSelected] = useLexicalNodeSelection(nodeKey);

	const handleRemove = () => {
		editor.update(() => {
			const node = $getNodeByKey(nodeKey);
			if (node instanceof SkillNode) {
				node.remove();
			}
		});
	};

	return (
		<SkillChip
			skillName={skillName}
			skillDescription={skillDescription}
			isSelected={isSelected}
			onRemove={handleRemove}
		/>
	);
});
SkillChipWrapper.displayName = "SkillChipWrapper";

export function $createSkillNode(
	skillName: string,
	skillDescription: string,
): SkillNode {
	return new SkillNode(skillName, skillDescription);
}
