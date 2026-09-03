import type { FC, ReactNode } from "react";

interface SpendSectionHeaderProps {
	title: string;
	description?: string;
	actions?: ReactNode;
}

export const SpendSectionHeader: FC<SpendSectionHeaderProps> = ({
	title,
	description,
	actions,
}) => {
	return (
		<div className="flex items-start justify-between gap-4">
			<div className="min-w-0 flex-1">
				<h2 className="m-0 text-xl font-semibold leading-7 text-content-primary">
					{title}
				</h2>
				{description && (
					<p className="m-0 mt-3 text-sm font-medium leading-6 text-content-secondary">
						{description}
					</p>
				)}
			</div>
			{actions}
		</div>
	);
};
