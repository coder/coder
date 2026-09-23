import { cn } from "cn";
import { PanelLeftIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import { useSidebarContext } from "./SidebarContext";

/**
 * Icon button that collapses or expands the sidebar. Sized to a 40px
 * square so it lines up with the nav rows; when expanded it bleeds 8px
 * into the header padding so the icon ends 16px from the sidebar edge.
 */
const SidebarCollapseToggle: FC = () => {
	const { collapsed, toggle } = useSidebarContext();
	return (
		<Button
			variant="subtle"
			size="icon-lg"
			onClick={toggle}
			aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
			className={cn(
				"shrink-0 hover:bg-surface-secondary [&>svg]:size-4! [&>svg]:p-0",
				!collapsed && "-mr-2",
			)}
		>
			<PanelLeftIcon />
		</Button>
	);
};

interface SidebarHeaderProps {
	/**
	 * Leading content, such as a title. It sits in a 40px row and should
	 * bleed 4px left (`-ml-1`) with 8px of inner padding if it has a hover
	 * surface, so its icon lands 16px from the sidebar edge.
	 */
	children?: ReactNode;
}

/**
 * Pinned 56px header for collapsible settings sidebars: leading content
 * beside the collapse toggle. When the sidebar is collapsed only the
 * toggle renders, placed at the left where the rail's icons are centered
 * (the panel stays 240px wide and is clipped to the 64px rail).
 */
export const SidebarHeader: FC<SidebarHeaderProps> = ({ children }) => {
	const { collapsed } = useSidebarContext();

	if (collapsed) {
		return (
			<div className="px-3 py-2">
				<SidebarCollapseToggle />
			</div>
		);
	}

	return (
		<div className="flex h-14 items-center gap-2 px-3 py-2">
			<div className="flex min-w-0 flex-1 items-center">{children}</div>
			<SidebarCollapseToggle />
		</div>
	);
};

/** Plain text title for the sidebar header. */
export const SidebarHeaderTitle: FC<{ children: ReactNode }> = ({
	children,
}) => (
	<span className="truncate pl-1 text-sm font-medium text-content-primary">
		{children}
	</span>
);
