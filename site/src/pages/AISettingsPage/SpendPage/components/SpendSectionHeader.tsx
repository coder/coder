import type { FC } from "react";

interface SpendSectionHeaderProps {
	title: string;
	description?: string;
}

export const SpendSectionHeader: FC<SpendSectionHeaderProps> = ({
	title,
	description,
}) => {
	return (
		<div>
			<h2 className="m-0 text-xl font-semibold leading-7 text-content-primary">
				{title}
			</h2>
			{description && (
				<p className="m-0 mt-3 text-sm font-medium leading-6 text-content-secondary">
					{description}
				</p>
			)}
		</div>
	);
};
