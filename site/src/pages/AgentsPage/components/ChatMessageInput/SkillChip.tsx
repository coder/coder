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
		<ZapIcon className="size-3.5 shrink-0 text-content-secondary" />
		<span className="min-w-0 truncate">{alias}</span>
	</>
);

/** A skill invoked by a sent message. */
export const SkillChip: React.FC<SkillChipProps> = ({
	source,
	name,
	className,
}) => (
	<span className={cn(fileReferenceChipVariants(), "pl-1", className)}>
		<SkillChipLabel alias={getSkillAlias(source, name)} />
	</span>
);

/** A skill attached to the message being composed. */
export const EditableSkillChip: React.FC<
	SkillChipProps & { selected: boolean; onRemove: () => void }
> = ({ source, name, selected, onRemove, className }) => {
	const alias = getSkillAlias(source, name);
	return (
		<span
			className={cn(
				fileReferenceChipVariants({ selected }),
				"pl-1 text-content-primary",
				!selected && "border-border-secondary bg-surface-tertiary",
				className,
			)}
			contentEditable={false}
		>
			<SkillChipLabel alias={alias} />
			<ChipRemoveButton label={`Remove skill ${alias}`} onRemove={onRemove} />
		</span>
	);
};
