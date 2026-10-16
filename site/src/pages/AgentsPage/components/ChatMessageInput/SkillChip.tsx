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
		<ZapIcon className="ml-0.5 size-3.5 shrink-0 text-content-secondary" />
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

/** A skill attached to the message being composed. */
export const EditableSkillChip: React.FC<
	SkillChipProps & { onRemove: () => void }
> = ({ source, name, onRemove, className }) => {
	const alias = getSkillAlias(source, name);
	return (
		<span
			className={cn(fileReferenceChipVariants({ editable: true }), className)}
			contentEditable={false}
		>
			<SkillChipLabel alias={alias} />
			<ChipRemoveButton label={`Remove skill ${alias}`} onRemove={onRemove} />
		</span>
	);
};
