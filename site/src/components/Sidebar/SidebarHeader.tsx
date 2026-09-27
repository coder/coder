import { cn } from "cn";
import { PanelLeftIcon } from "lucide-react";
import type { FC, ReactNode } from "react";
import { Button } from "#/components/Button/Button";
import { useSidebarContext } from "./SidebarContext";

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

type SidebarHeaderProps = {
	/** Leading content, such as a title. */
	children?: ReactNode;
};

/** Sidebar header with the collapse toggle. Collapsed, only the toggle shows. */
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

/** Text title for SidebarHeader. */
export const SidebarHeaderTitle: FC<{ children: ReactNode }> = ({
	children,
}) => (
	<span className="truncate pl-1 text-sm font-medium text-content-primary">
		{children}
	</span>
);
