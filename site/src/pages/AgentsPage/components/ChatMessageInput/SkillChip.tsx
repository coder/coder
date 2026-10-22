import { cn } from "cn";
import { ZapIcon } from "lucide-react";
import { getSkillAlias, type SkillSource } from "../../utils/skillAliases";
import {
	ChipRemoveButton,
	fileReferenceChipVariants,
} from "./FileReferenceChip";

type SkillChipProps = {
	source: SkillSource;
	name: string;
	className?: string;
};

/** Atomic Lexical decorator for SkillInvocationNode; arrows skip it, Backspace or its button removes it. */
export const EditableSkillChip: React.FC<
	SkillChipProps & { onRemove: () => void }
> = ({ source, name, onRemove, className }) => {
	const alias = getSkillAlias(source, name);
	return (
		<span
			className={cn(fileReferenceChipVariants({ editable: true }), className)}
			contentEditable={false}
		>
			<ZapIcon className="ml-0.5 size-icon-xs shrink-0 text-content-secondary" />
			<span className="min-w-0 truncate" title={alias}>
				{alias}
			</span>
			<ChipRemoveButton label={`Remove skill ${alias}`} onRemove={onRemove} />
		</span>
	);
};
