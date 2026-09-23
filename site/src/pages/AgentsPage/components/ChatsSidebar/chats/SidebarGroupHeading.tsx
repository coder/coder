import type { FC, ReactNode } from "react";

type SidebarGroupHeadingProps = {
	readonly label: string;
	readonly action?: ReactNode;
};

/**
 * Non-collapsible heading for a top-level sidebar group such as Projects or
 * Chats. Its left inset matches the chevron slot of the rows below so labels
 * line up with row content.
 */
export const SidebarGroupHeading: FC<SidebarGroupHeadingProps> = ({
	label,
	action,
}) => (
	<div className="mb-1 flex h-7 items-center pl-1 pr-1 text-xs font-medium text-content-secondary">
		<h2 className="m-0 min-w-0 flex-1 truncate text-xs font-medium leading-none">
			{label}
		</h2>
		{action}
	</div>
);
