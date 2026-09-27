import type { FC, ReactNode } from "react";

type RightPanelEmptyStateProps = {
	icon?: ReactNode;
	title: string;
	description?: string;
	action?: ReactNode;
};

export const RightPanelEmptyState: FC<RightPanelEmptyStateProps> = ({
	icon,
	title,
	description,
	action,
}) => (
	<div className="flex h-full min-h-0 flex-col items-center justify-center gap-3 px-6 text-center">
		{icon && (
			<div className="flex size-10 items-center justify-center rounded-lg bg-surface-secondary text-content-secondary [&>svg]:size-5">
				{icon}
			</div>
		)}
		<div className="flex flex-col gap-1">
			<p className="m-0 text-sm font-medium text-content-primary">{title}</p>
			{description && (
				<p className="m-0 text-xs text-content-secondary">{description}</p>
			)}
		</div>
		{action}
	</div>
);
