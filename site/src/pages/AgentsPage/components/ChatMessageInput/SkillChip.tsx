import { cn } from "cn";
import { XIcon, ZapIcon } from "lucide-react";
import { getSkillAlias, type SkillSource } from "../../utils/skillAliases";
import { fileReferenceChipVariants } from "./FileReferenceChip";

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

/** A skill attached to the message being composed. */
export const EditableSkillChip: React.FC<
	SkillChipProps & { selected: boolean; onRemove: () => void }
> = ({ source, name, selected, onRemove, className }) => {
	const alias = getSkillAlias(source, name);
	return (
		<span
			className={cn(
				fileReferenceChipVariants({ selected }),
				"border-border-secondary bg-surface-tertiary pl-1 text-content-primary",
				className,
			)}
			contentEditable={false}
		>
			<SkillChipLabel alias={alias} />
			<button
				type="button"
				className="ml-0.5 inline-flex size-3.5 shrink-0 cursor-pointer items-center justify-center rounded border-0 bg-transparent p-0 text-content-secondary transition-colors hover:bg-surface-quaternary hover:text-content-primary"
				onClick={(e) => {
					e.preventDefault();
					e.stopPropagation();
					onRemove();
				}}
				aria-label={`Remove skill ${alias}`}
				tabIndex={-1}
			>
				<XIcon className="size-2.5" />
			</button>
		</span>
	);
};
