import { cn } from "cn";
import type { FC, ReactNode } from "react";

type SidebarGroupProps = {
	label: string;
	/** Whether this group contains the current route. */
	active?: boolean;
	children: ReactNode;
};

/** Static heading with its links along a vertical line. */
export const SidebarGroup: FC<SidebarGroupProps> = ({
	label,
	active = false,
	children,
}) => {
	return (
		<div className="flex flex-col">
			<div
				className={cn(
					"flex h-8 items-center pl-1 text-sm font-medium text-content-secondary whitespace-nowrap",
					active && "text-content-primary",
				)}
			>
				{label}
			</div>
			<div className="ml-1 flex flex-col border-0 border-l border-solid border-border pl-1">
				{children}
			</div>
		</div>
	);
};
