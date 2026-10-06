import { cn } from "cn";
import { ChevronDownIcon } from "lucide-react";

type SidebarSectionHeaderProps = {
	readonly title: React.ReactNode;
	readonly actions?: React.ReactNode;
	/** Makes the title a button that shows or hides the section's content. */
	readonly collapsible?: {
		readonly expanded: boolean;
		readonly onToggle: () => void;
	};
};

/** Heading for a top-level sidebar section, with its actions on the right. */
export const SidebarSectionHeader: React.FC<SidebarSectionHeaderProps> = ({
	title,
	actions,
	collapsible,
}) => (
	<div className="mx-2 pt-6 mb-1.5">
		<div className="ml-2.5 flex h-7 items-center justify-between">
			<h2 className="m-0 text-sm font-normal leading-6 text-content-secondary">
				{collapsible ? (
					<button
						type="button"
						className="flex cursor-pointer appearance-none items-center gap-1 rounded-md border-0 bg-transparent p-0 font-sans text-sm leading-6 text-current hover:text-content-primary focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link"
						aria-expanded={collapsible.expanded}
						onClick={collapsible.onToggle}
					>
						{title}
						<ChevronDownIcon
							aria-hidden="true"
							className={cn(
								"size-3.5 transition-transform",
								!collapsible.expanded && "-rotate-90",
							)}
						/>
					</button>
				) : (
					title
				)}
			</h2>
			<div className="flex items-center gap-1">{actions}</div>
		</div>
	</div>
);
