import { cva } from "class-variance-authority";
import type { FC, ReactNode } from "react";
import { cn } from "#/utils/cn";

const headerVariants = cva("flex items-start justify-between gap-4");
const titleVariants = cva(
	"m-0 text-xl font-semibold leading-7 text-content-primary",
);
const descriptionVariants = cva(
	"m-0 mt-3 text-sm font-medium leading-6 text-content-secondary",
);

interface SpendSectionHeaderProps {
	title: string;
	description?: string;
	actions?: ReactNode;
	className?: string;
}

export const SpendSectionHeader: FC<SpendSectionHeaderProps> = ({
	title,
	description,
	actions,
	className,
}) => {
	return (
		<div className={cn(headerVariants(), className)}>
			<div className="min-w-0 flex-1">
				<h2 className={titleVariants()}>{title}</h2>
				{description && <p className={descriptionVariants()}>{description}</p>}
			</div>
			{actions}
		</div>
	);
};
