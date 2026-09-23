import { cn } from "cn";
import type { FC, ReactNode } from "react";

interface SidebarGroupProps {
	label: string;
	/** Whether this group contains the current route. */
	active?: boolean;
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
	active = false,
	children,
}) => {
	return (
		<div className="flex flex-col">
			{/* A 32px row whose label sits at the same 4px inset as the rows'
			    text. */}
			<div
				className={cn(
					"flex h-8 items-center pl-1 text-sm font-medium text-content-secondary whitespace-nowrap",
					active && "text-content-primary",
				)}
			>
				{label}
			</div>
			{/* The line sits at the label's left edge. Items stack flush
			    against it and each other. */}
			<div className="ml-1 flex flex-col border-0 border-l border-solid border-border pl-1">
				{children}
			</div>
		</div>
	);
};
