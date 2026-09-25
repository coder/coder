import type { FC } from "react";
import { useLocation } from "react-router";
import type { Workspace } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { useSidebarContext } from "#/components/Sidebar/SidebarContext";
import { SidebarGroup } from "#/components/Sidebar/SidebarGroup";
import {
	SidebarHeader,
	SidebarHeaderTitle,
} from "#/components/Sidebar/SidebarHeader";
import { SidebarNavLink } from "#/components/Sidebar/SidebarNavLink";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type WorkspaceSettingsLink = {
	label: string;
	/** Path under the settings base; empty for the index. */
	segment: string;
	visible: boolean;
};

type WorkspaceSettingsGroup = {
	label: string;
	items: WorkspaceSettingsLink[];
};

/** Pinned header for the workspace settings sidebar. */
export const WorkspaceSettingsSidebarHeader: FC = () => (
	<SidebarHeader>
		<SidebarHeaderTitle>Workspace settings</SidebarHeaderTitle>
	</SidebarHeader>
);

type WorkspaceSettingsSidebarViewProps = {
	workspace: Workspace;
	canShareWorkspace: boolean;
};

/** Workspace settings navigation. Collapsed, it shows only the template icon. */
export const WorkspaceSettingsSidebarView: FC<
	WorkspaceSettingsSidebarViewProps
> = ({ workspace, canShareWorkspace }) => {
	const { pathname } = useLocation();
	const { collapsed, expand } = useSidebarContext();
	const base = `/@${workspace.owner_name}/${workspace.name}/settings`;
	const templateName =
		workspace.template_display_name || workspace.template_name;

	const groups: WorkspaceSettingsGroup[] = [
		{
			label: "Workspace",
			items: [
				{ label: "General", segment: "", visible: true },
				{ label: "Parameters", segment: "parameters", visible: true },
				{ label: "Schedule", segment: "schedule", visible: true },
			],
		},
		{
			label: "Access",
			items: [
				{ label: "Sharing", segment: "sharing", visible: canShareWorkspace },
			],
		},
	];

	const hrefFor = (segment: string) => (segment ? `${base}/${segment}` : base);
	const isActive = (segment: string) =>
		segment
			? pathname.startsWith(hrefFor(segment))
			: pathname === base || pathname === `${base}/`;

	const icon = (
		<Avatar
			variant="icon"
			size="lg"
			fallback={templateName}
			src={workspace.template_icon}
		/>
	);

	if (collapsed) {
		return (
			<TooltipProvider>
				<Tooltip delayDuration={0}>
					<TooltipTrigger asChild>
						<button
							type="button"
							onClick={expand}
							aria-label={workspace.name}
							className="flex items-center justify-center w-10 h-10 rounded-md cursor-pointer bg-transparent border-none p-0 hover:bg-surface-secondary"
						>
							{icon}
						</button>
					</TooltipTrigger>
					<TooltipContent side="right">{workspace.name}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		);
	}

	return (
		<div className="flex flex-col gap-4">
			<div className="flex items-center gap-2 px-1 py-1">
				{icon}
				<div className="flex min-w-0 flex-1 flex-col">
					<span className="truncate text-sm text-content-primary">
						{workspace.name}
					</span>
					<span className="truncate text-xs text-content-secondary">
						{workspace.owner_name}
					</span>
				</div>
			</div>
			{groups
				.filter((group) => group.items.some((item) => item.visible))
				.map((group) => (
					<SidebarGroup
						key={group.label}
						label={group.label}
						active={group.items.some(
							(item) => item.visible && isActive(item.segment),
						)}
					>
						{group.items
							.filter((item) => item.visible)
							.map((item) => (
								<SidebarNavLink
									key={item.segment}
									href={hrefFor(item.segment)}
									end={item.segment === ""}
								>
									{item.label}
								</SidebarNavLink>
							))}
					</SidebarGroup>
				))}
		</div>
	);
};
