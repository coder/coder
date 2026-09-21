import { cn } from "cn";
import { LayoutGridIcon, PlusIcon, SquareTerminalIcon } from "lucide-react";
import { type FC, type ReactNode, useId, useState } from "react";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
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

type WorkspaceOpenMenuProps = {
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	isRunning: boolean;
	onOpenWorkspaceApp: (app: WorkspaceApp) => void;
	onOpenCommandApp: (app: WorkspaceApp) => void;
	onOpenPort: (selection: PortSelection) => void;
	trigger: ReactNode;
};

/** Lists the agent's apps and forwarded ports; selecting one opens it. */
const WorkspaceOpenMenu: FC<WorkspaceOpenMenuProps> = ({
	workspace,
	agent,
	host,
	isRunning,
	onOpenWorkspaceApp,
	onOpenCommandApp,
	onOpenPort,
	trigger,
}) => {
	const [open, setOpen] = useState(false);
	// agent-browser already has the built-in Browser tab.
	const userApps = agent.apps.filter(
		(app) => !app.hidden && app.slug !== AGENT_BROWSER_APP_SLUG,
	);
	const showPorts = canShowPortForwarding(agent, host);

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				side="bottom"
				className="w-52 p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs [&_img]:size-3.5! [&_svg]:size-3.5!"
			>
				{userApps.length === 0 && !showPorts && (
					<p className="m-0 px-2 py-2 text-center text-xs text-content-tertiary">
						This workspace has no apps.
					</p>
				)}
				{userApps.map((app) => {
					const icon = app.icon ? (
						<ExternalImage src={app.icon} alt="" className="rounded-sm" />
					) : app.command ? (
						<SquareTerminalIcon />
					) : (
						<LayoutGridIcon />
					);
					if (app.command) {
						return (
							<DropdownMenuItem
								key={app.id}
								onSelect={() => onOpenCommandApp(app)}
								disabled={!isRunning}
							>
								{icon}
								{app.display_name ?? app.slug}
							</DropdownMenuItem>
						);
					}
					if (isWorkspaceAppEmbeddable(app)) {
						return (
							<DropdownMenuItem
								key={app.id}
								onSelect={() => onOpenWorkspaceApp(app)}
								disabled={!isRunning}
							>
								{icon}
								{app.display_name ?? app.slug}
							</DropdownMenuItem>
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
				{showPorts && (
					<>
						{userApps.length > 0 && <DropdownMenuSeparator className="my-1" />}
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

type WorkspaceTabPanelProps = Omit<WorkspaceOpenMenuProps, "trigger"> & {
	previews: readonly WorkspacePreviewRightPanelTab[];
	activePreviewId: string | null;
	/** Whether the right panel is open with the Workspace tab selected. */
	isVisible: boolean;
	onActivePreviewChange: (previewId: string) => void;
	onClosePreview: (previewId: string) => void;
};

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
	previews,
	activePreviewId,
	isVisible,
	onActivePreviewChange,
	onClosePreview,
	...menuProps
}) => {
	const idPrefix = useId();

	if (previews.length === 0) {
		return (
			<RightPanelEmptyState
				icon={<LayoutGridIcon />}
				title="Nothing open"
				description="Open a workspace app or forwarded port to preview it here."
				action={
					<WorkspaceOpenMenu
						{...menuProps}
						trigger={
							<Button variant="outline" size="sm">
								<PlusIcon />
								Open app or port
							</Button>
						}
					/>
				}
			/>
		);
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<SubTabStrip
				label="Workspace previews"
				idPrefix={idPrefix}
				tabs={previews.map((preview) => ({
					id: preview.id,
					label: preview.label,
					onClose: () => onClosePreview(preview.id),
				}))}
				activeTabId={activePreviewId}
				onActiveTabChange={onActivePreviewChange}
				trailing={
					<WorkspaceOpenMenu
						{...menuProps}
						trigger={
							<Button
								variant="outline"
								size="icon"
								aria-label="Open app or port"
								title="Open app or port"
								className="size-7 shrink-0 p-0 text-content-secondary hover:text-content-primary [&>svg]:size-3.5"
							>
								<PlusIcon />
							</Button>
						}
					/>
				}
			/>
			<div className="relative flex min-h-0 flex-1 flex-col">
				{previews.map((preview) => {
					const isActive = preview.id === activePreviewId;
					return (
						<div
							key={preview.id}
							role="tabpanel"
							aria-labelledby={getSubTabElementId(idPrefix, preview.id)}
							className={cn(
								"flex min-h-0 flex-1 flex-col",
								!isActive && "invisible absolute inset-0",
							)}
							inert={!isActive}
						>
							<PreviewContent
								preview={preview}
								workspace={menuProps.workspace}
								agent={menuProps.agent}
								host={menuProps.host}
								isActive={isVisible && isActive}
							/>
						</div>
					);
				})}
			</div>
		</div>
	);
};
