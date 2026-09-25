import { cn } from "cn";
import {
	ChevronDownIcon,
	LayoutGridIcon,
	NetworkIcon,
	PlusIcon,
	SquareTerminalIcon,
	XIcon,
} from "lucide-react";
import { type FC, type ReactNode, useId, useState } from "react";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	AGENT_BROWSER_APP_SLUG,
	isWorkspaceAppEmbeddable,
} from "#/modules/apps/apps";
import { WorkspaceAppFrame } from "#/modules/apps/WorkspaceAppFrame";
import { findWorkspaceAppWithAgent } from "#/modules/apps/workspaceApps";
import { AppLink } from "#/modules/resources/AppLink/AppLink";
import {
	canShowPortForwarding,
	usePortsData,
} from "#/modules/resources/usePortsData";
import type {
	PortSelection,
	WorkspacePreviewRightPanelTab,
} from "../../utils/rightPanelTabs";
import { PortsMenuItem } from "../WorkspacePillPorts";
import { PortPreviewPanel } from "./PortPreviewPanel";
import { RightPanelEmptyState } from "./RightPanelEmptyState";
import { getSubTabElementId, SubTabStrip } from "./SubTabStrip";
import { useOverflows } from "./useOverflows";

/** Apps the Workspace tab offers; agent-browser has its own Browser tab. */
function listWorkspaceApps(agent: WorkspaceAgent): WorkspaceApp[] {
	return agent.apps.filter(
		(app) => !app.hidden && app.slug !== AGENT_BROWSER_APP_SLUG,
	);
}

/** Whether the agent exposes anything for the Workspace tab to show. */
export function hasWorkspaceTabContent(
	agent: WorkspaceAgent,
	host: string,
): boolean {
	return (
		listWorkspaceApps(agent).length > 0 || canShowPortForwarding(agent, host)
	);
}

// usePortsData requires a workspace and agent, which are optional props on the
// parent control, so the hook lives in this conditionally rendered component.
const AgentPortsSubMenu: FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isOpen: boolean;
	isRunning: boolean;
	onPortSelect: (selection: PortSelection) => void;
}> = ({ workspace, agent, host, isOpen, isRunning, onPortSelect }) => {
	const portsData = usePortsData(
		workspace,
		agent,
		isOpen && agent.status === "connected",
	);
	return (
		<PortsMenuItem
			workspace={workspace}
			agent={agent}
			host={host}
			portsData={portsData}
			isRunning={isRunning}
			isBelowMd={false}
			focusOnMount={false}
			onPortSelect={onPortSelect}
		/>
	);
};

const appIcon = (app: WorkspaceApp): ReactNode => {
	if (app.icon) {
		return <ExternalImage src={app.icon} alt="" className="rounded-sm" />;
	}
	return app.command ? <SquareTerminalIcon /> : <LayoutGridIcon />;
};

type WorkspaceTabPanelProps = {
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isRunning: boolean;
	previews: readonly WorkspacePreviewRightPanelTab[];
	activePreviewId: string | null;
	/** Whether the right panel is open with the Workspace tab selected. */
	isVisible: boolean;
	onActivePreviewChange: (previewId: string) => void;
	onClosePreview: (previewId: string) => void;
	onOpenWorkspaceApp: (app: WorkspaceApp) => void;
	onOpenCommandApp: (app: WorkspaceApp) => void;
	onOpenPort: (selection: PortSelection) => void;
};

type WorkspaceOpenMenuProps = Omit<
	WorkspaceTabPanelProps,
	"isVisible" | "onClosePreview"
> & {
	/** Menu trigger; receives the open state so it can rotate a chevron. */
	trigger: (open: boolean) => ReactNode;
};

/**
 * Lists the agent's apps and ports. Checked apps are open as previews;
 * selecting one shows it, selecting an unchecked one opens it.
 */
const WorkspaceOpenMenu: FC<WorkspaceOpenMenuProps> = ({
	workspace,
	agent,
	host,
	isRunning,
	previews,
	onActivePreviewChange,
	onOpenWorkspaceApp,
	onOpenCommandApp,
	onOpenPort,
	trigger,
}) => {
	const [open, setOpen] = useState(false);
	const userApps = listWorkspaceApps(agent);
	const showPorts = canShowPortForwarding(agent, host);
	const portPreviews = previews.filter((preview) => preview.kind === "port");

	const handleSelectApp = (app: WorkspaceApp) => {
		const preview = previews.find(
			(candidate) =>
				candidate.kind === "workspace_app" && candidate.appId === app.id,
		);
		if (preview) {
			onActivePreviewChange(preview.id);
		} else {
			onOpenWorkspaceApp(app);
		}
	};

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>{trigger(open)}</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				side="bottom"
				className="w-56 p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs [&_img]:size-3.5! [&_svg]:size-3.5!"
			>
				{userApps.map((app) => {
					if (app.command) {
						return (
							<DropdownMenuItem
								key={app.id}
								onSelect={() => onOpenCommandApp(app)}
								disabled={!isRunning}
							>
								{appIcon(app)}
								{app.display_name ?? app.slug}
							</DropdownMenuItem>
						);
					}
					if (isWorkspaceAppEmbeddable(app)) {
						const isOpen = previews.some(
							(preview) =>
								preview.kind === "workspace_app" && preview.appId === app.id,
						);
						return (
							<DropdownMenuCheckboxItem
								key={app.id}
								checked={isOpen}
								onSelect={() => handleSelectApp(app)}
								disabled={!isRunning && !isOpen}
							>
								{appIcon(app)}
								{app.display_name ?? app.slug}
							</DropdownMenuCheckboxItem>
						);
					}
					return (
						<AppLink
							key={app.id}
							workspace={workspace}
							agent={agent}
							app={app}
							grouped
						/>
					);
				})}
				{portPreviews.map((preview) => (
					<DropdownMenuCheckboxItem
						key={preview.id}
						checked
						onSelect={() => onActivePreviewChange(preview.id)}
					>
						<NetworkIcon />
						{preview.label}
					</DropdownMenuCheckboxItem>
				))}
				{showPorts && (
					<>
						{(userApps.length > 0 || portPreviews.length > 0) && (
							<DropdownMenuSeparator className="my-1" />
						)}
						<AgentPortsSubMenu
							workspace={workspace}
							agent={agent}
							host={host}
							isOpen={open}
							isRunning={isRunning}
							onPortSelect={onOpenPort}
						/>
					</>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};

const addPreviewTrigger = () => (
	<Button
		variant="outline"
		size="icon"
		aria-label="Open an app or port"
		title="Open an app or port"
		className="size-7 shrink-0 p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3.5"
	>
		<PlusIcon />
	</Button>
);

const PreviewContent: FC<{
	preview: WorkspacePreviewRightPanelTab;
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isActive: boolean;
}> = ({ preview, workspace, agent, host, isActive }) => {
	if (preview.kind === "port") {
		return (
			<PortPreviewPanel
				workspace={workspace}
				agent={agent}
				host={host}
				tab={preview}
			/>
		);
	}
	const app = findWorkspaceAppWithAgent(
		workspace,
		preview.agentId,
		preview.appId,
	);
	if (!app || !isWorkspaceAppEmbeddable(app)) {
		return (
			<RightPanelEmptyState title="This app is no longer available as a preview." />
		);
	}
	return (
		<WorkspaceAppFrame workspace={workspace} app={app} active={isActive} />
	);
};

export const WorkspaceTabPanel: FC<WorkspaceTabPanelProps> = ({
	isVisible,
	onClosePreview,
	...menuProps
}) => {
	const { workspace, agent, host, previews, activePreviewId } = menuProps;
	const idPrefix = useId();
	// A hidden copy of the chip row measures whether every chip fits on one
	// line; when it does not, the strip collapses into a selector.
	const { ref: measureRef, overflows } = useOverflows(
		previews.map((preview) => preview.label).join("\n"),
	);

	if (previews.length === 0) {
		return (
			<div className="flex h-full min-h-0 flex-col items-center justify-center px-6 text-center">
				<WorkspaceOpenMenu
					{...menuProps}
					trigger={() => (
						<Button variant="outline" size="sm">
							<PlusIcon />
							Open an app or port
						</Button>
					)}
				/>
			</div>
		);
	}

	const chipTabs = previews.map((preview) => ({
		id: preview.id,
		label: preview.label,
		onClose: () => onClosePreview(preview.id),
	}));
	const activePreview = previews.find(
		(preview) => preview.id === activePreviewId,
	);

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="relative flex shrink-0 items-center border-0 border-b border-solid border-border-default px-3 py-2">
				<div
					ref={measureRef}
					aria-hidden
					inert
					className="invisible absolute inset-x-3 top-0 flex items-center gap-1.5 overflow-hidden"
				>
					<SubTabStrip
						label="Workspace previews"
						idPrefix={`${idPrefix}-measure`}
						tabs={chipTabs}
						activeTabId={activePreviewId}
						onActiveTabChange={() => {}}
						className="w-max flex-none overflow-visible"
					/>
					{addPreviewTrigger()}
				</div>
				{overflows ? (
					<div className="flex min-w-0 items-center gap-1.5">
						<WorkspaceOpenMenu
							{...menuProps}
							trigger={(open) => (
								<Button
									variant="outline"
									size="sm"
									className="h-8 max-w-full min-w-0 justify-between gap-2 px-3 text-xs"
								>
									<span className="truncate">
										{activePreview?.label ?? "Open an app or port"}
									</span>
									<ChevronDownIcon
										className={cn(
											"size-3.5 shrink-0 transition-transform",
											open && "rotate-180",
										)}
									/>
								</Button>
							)}
						/>
						{activePreview && (
							<Button
								variant="outline"
								size="icon"
								onClick={() => onClosePreview(activePreview.id)}
								aria-label={`Close ${activePreview.label}`}
								title={`Close ${activePreview.label}`}
								className="size-8 shrink-0 p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3.5"
							>
								<XIcon />
							</Button>
						)}
					</div>
				) : (
					<SubTabStrip
						label="Workspace previews"
						idPrefix={idPrefix}
						tabs={chipTabs}
						activeTabId={activePreviewId}
						onActiveTabChange={menuProps.onActivePreviewChange}
						trailing={
							<WorkspaceOpenMenu {...menuProps} trigger={addPreviewTrigger} />
						}
					/>
				)}
			</div>
			<div className="relative flex min-h-0 flex-1 flex-col">
				{previews.map((preview) => {
					const isActive = preview.id === activePreviewId;
					// The selector is a menu, not a tablist, so the panels only carry
					// tab semantics while the chips are shown.
					const tabPanelProps = overflows
						? {}
						: {
								role: "tabpanel",
								"aria-labelledby": getSubTabElementId(idPrefix, preview.id),
							};
					return (
						<div
							key={preview.id}
							{...tabPanelProps}
							className={cn(
								"flex min-h-0 flex-1 flex-col",
								!isActive && "invisible absolute inset-0",
							)}
							inert={!isActive}
						>
							<PreviewContent
								preview={preview}
								workspace={workspace}
								agent={agent}
								host={host}
								isActive={isVisible && isActive}
							/>
						</div>
					);
				})}
			</div>
		</div>
	);
};
