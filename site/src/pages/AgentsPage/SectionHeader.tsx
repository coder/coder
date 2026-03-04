import type { FC, ReactNode } from "react";

type SectionHeaderProps = {
	label: string;
	description?: string;
	action?: ReactNode;
};

export const SectionHeader: FC<SectionHeaderProps> = ({
	label,
	description,
	action,
}) => (
	<>
		<div className="flex items-start justify-between gap-4">
			<div>
				<h2 className="m-0 text-lg font-medium text-content-primary">
					{label}
				</h2>
				{description && (
					<p className="m-0 text-sm text-content-secondary">{description}</p>
				)}
			</div>
			{action}
		</div>
		<hr className="my-4 border-0 border-t border-solid border-border" />
	</>
);
