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

const SkillChipLabel: React.FC<{ alias: string }> = ({ alias }) => (
	<>
		<ZapIcon className="ml-0.5 size-icon-xs shrink-0 text-content-secondary" />
		<span className="min-w-0 truncate" title={alias}>
			{alias}
		</span>
	</>
);

/** A display-only skill chip in sent and queued messages. */
export const SkillChip: React.FC<SkillChipProps> = ({
	source,
	name,
	className,
}) => (
	<span className={cn(fileReferenceChipVariants(), className)}>
		<SkillChipLabel alias={getSkillAlias(source, name)} />
	</span>
);

/** Atomic Lexical decorator for SkillInvocationNode; arrows skip it, Backspace or its button removes it. */
export const EditableSkillChip: React.FC<
	SkillChipProps & { "data-selected": boolean; onRemove: () => void }
> = ({ source, name, onRemove, className, ...props }) => {
	const alias = getSkillAlias(source, name);
	return (
		<span
			className={cn(
				fileReferenceChipVariants({ editable: true }),
				"data-[selected=true]:border-content-link data-[selected=true]:bg-content-link/10 data-[selected=true]:text-content-primary data-[selected=true]:ring-1 data-[selected=true]:ring-content-link/40",
				className,
			)}
			contentEditable={false}
			{...props}
		>
			<SkillChipLabel alias={alias} />
			<ChipRemoveButton label={`Remove skill ${alias}`} onRemove={onRemove} />
		</span>
	);
};
