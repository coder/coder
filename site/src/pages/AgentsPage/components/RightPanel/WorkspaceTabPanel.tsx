import { cn } from "cn";
import {
	ChevronDownIcon,
	LayoutGridIcon,
	NetworkIcon,
	SquareTerminalIcon,
} from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
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

/**
 * Workspace tab: a selector over the agent's apps and ports. Checked items
 * are open as previews; the trigger shows the one currently displayed.
 */
export const WorkspaceTabPanel: FC<WorkspaceTabPanelProps> = ({
	workspace,
	agent,
	host,
	isRunning,
	previews,
	activePreviewId,
	isVisible,
	onActivePreviewChange,
	onClosePreview,
	onOpenWorkspaceApp,
	onOpenCommandApp,
	onOpenPort,
}) => {
	const [open, setOpen] = useState(false);
	// agent-browser already has the built-in Browser tab.
	const userApps = agent.apps.filter(
		(app) => !app.hidden && app.slug !== AGENT_BROWSER_APP_SLUG,
	);
	const showPorts = canShowPortForwarding(agent, host);
	const activePreview = previews.find(
		(preview) => preview.id === activePreviewId,
	);
	const portPreviews = previews.filter((preview) => preview.kind === "port");

	// Selecting an open preview shows it; selecting the one already shown
	// closes it, so the checkbox doubles as the close control.
	const togglePreview = (preview: WorkspacePreviewRightPanelTab) => {
		if (preview.id === activePreviewId) {
			onClosePreview(preview.id);
		} else {
			onActivePreviewChange(preview.id);
		}
	};

	const handleSelectApp = (app: WorkspaceApp) => {
		const preview = previews.find(
			(candidate) =>
				candidate.kind === "workspace_app" && candidate.appId === app.id,
		);
		if (preview) {
			togglePreview(preview);
		} else {
			onOpenWorkspaceApp(app);
		}
	};

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex shrink-0 items-center border-0 border-b border-solid border-border-default px-3 py-2">
				<DropdownMenu open={open} onOpenChange={setOpen}>
					<DropdownMenuTrigger asChild>
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
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="start"
						side="bottom"
						className="w-56 p-1 [&_[role^=menuitem]]:py-1 [&_[role^=menuitem]]:text-xs [&_img]:size-3.5! [&_svg]:size-3.5!"
					>
						{userApps.length === 0 &&
							portPreviews.length === 0 &&
							!showPorts && (
								<p className="m-0 px-2 py-2 text-center text-xs text-content-tertiary">
									This workspace has no apps.
								</p>
							)}
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
										preview.kind === "workspace_app" &&
										preview.appId === app.id,
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
								onSelect={() => togglePreview(preview)}
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
			</div>
			<div className="relative flex min-h-0 flex-1 flex-col">
				{previews.length === 0 && (
					<RightPanelEmptyState
						icon={<LayoutGridIcon />}
						title="Nothing open"
						description="Pick a workspace app or forwarded port above to preview it here."
					/>
				)}
				{previews.map((preview) => {
					const isActive = preview.id === activePreviewId;
					return (
						<div
							key={preview.id}
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
