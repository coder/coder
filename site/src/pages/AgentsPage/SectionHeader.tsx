import type { FC, ReactNode } from "react";

interface SectionHeaderProps {
	label: string;
	description?: string;
	badge?: ReactNode;
	action?: ReactNode;
}

export const SectionHeader: FC<SectionHeaderProps> = ({
	label,
	description,
	badge,
	action,
}) => (
	<>
		<div className="flex items-start justify-between gap-4">
			<div>
				<div className="flex items-center gap-2">
					<h2 className="m-0 text-lg font-medium text-content-primary">
						{label}
					</h2>
					{badge}
				</div>
				{description && (
					<p className="m-0 mt-0.5 text-sm text-content-secondary">
						{description}
					</p>
				)}
			</div>
			{action}
		</div>
		<hr className="my-4 border-0 border-t border-solid border-border" />
	</>
);
