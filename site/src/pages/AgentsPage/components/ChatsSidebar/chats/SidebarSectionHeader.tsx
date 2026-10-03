type SidebarSectionHeaderProps = {
	readonly title: React.ReactNode;
	readonly actions?: React.ReactNode;
};

/** Heading for a top-level sidebar section, with its actions on the right. */
export const SidebarSectionHeader: React.FC<SidebarSectionHeaderProps> = ({
	title,
	actions,
}) => (
	<div className="mx-2 pt-6 mb-1.5">
		<div className="ml-2.5 flex h-7 items-center justify-between">
			<h2 className="m-0 text-sm font-normal leading-6 text-content-secondary">
				{title}
			</h2>
			<div className="flex items-center gap-1">{actions}</div>
		</div>
	</div>
);
