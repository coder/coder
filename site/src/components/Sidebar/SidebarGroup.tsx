import { cn } from "cn";
import type { ElementType, FC, ReactNode } from "react";

interface SidebarGroupProps {
	label: string;
	/**
	 * Icon shown before the label. Groups that stand in for a whole section
	 * (the stacked admin list, user settings) carry one so they line up with
	 * the flat icon links around them; groups inside a section do not.
	 */
	icon?: ElementType;
	/** Whether this group contains the current route. */
	active?: boolean;
	/**
	 * Content between the heading and the list, spanning the row width,
	 * such as the organization switcher.
	 */
	controls?: ReactNode;
	children: ReactNode;
}

/**
 * Always-expanded group of sidebar links under a static heading. The
 * heading is a label, not a control: it never navigates or collapses.
 * Children hang off a connecting line that starts at the heading's label
 * edge, and the heading lifts to the primary text color while one of its
 * links is the current page.
 */
export const SidebarGroup: FC<SidebarGroupProps> = ({
	label,
	icon: Icon,
	active = false,
	controls,
	children,
}) => {
	return (
		<div className="flex flex-col">
			{/* Icon groups are 40px rows with the icon inset 4px, matching the
			    flat icon links; plain groups are 32px rows whose label sits at
			    the same 4px inset. */}
			<div
				className={cn(
					"flex items-center gap-2 text-sm font-medium text-content-secondary whitespace-nowrap",
					Icon ? "h-10 px-1" : "h-8 pl-1",
					active && "text-content-primary",
				)}
			>
				{Icon && <Icon className="size-4 shrink-0" />}
				<span>{label}</span>
			</div>
			{controls && <div className="-ml-1 mr-1 py-2">{controls}</div>}
			{/* The line sits at the label's left edge: 4px inset plus, for icon
			    groups, the 16px icon and 8px gap. Items stack flush against it
			    and each other. */}
			<div
				className={cn(
					"flex flex-col pl-1 border-0 border-l border-solid border-border",
					Icon ? "ml-7" : "ml-1",
				)}
			>
				{children}
			</div>
		</div>
	);
};
