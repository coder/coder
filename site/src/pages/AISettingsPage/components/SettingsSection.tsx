import { useId } from "react";

type SettingsSectionProps = {
	title: string;
	description: string;
	actions?: React.ReactNode;
	children: React.ReactNode;
};

/**
 * A page section labelled by its heading. Actions, such as an organization
 * picker, render beside the heading.
 */
export const SettingsSection: React.FC<SettingsSectionProps> = ({
	title,
	description,
	actions,
	children,
}) => {
	const titleId = useId();

	return (
		<section aria-labelledby={titleId} className="flex flex-col gap-6">
			<div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
				<div>
					<h2 id={titleId} className="m-0 text-xl font-semibold">
						{title}
					</h2>
					<p className="mt-1 mb-0 text-sm text-content-secondary">
						{description}
					</p>
				</div>
				{actions}
			</div>
			{children}
		</section>
	);
};
