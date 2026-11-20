import { ExternalLinkIcon } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { useTabPanelState } from "../ChatsSidebar/tabs/SidebarTabView";
import { PreviewFrame } from "./PreviewFrame";

type PreviewPanelProps = {
	workspace: Workspace;
	agent: WorkspaceAgent;
	tab: Extract<UserRightPanelTab, { kind: "preview" }>;
};

/** Shows a port or file preview that the agent opened with `open_preview`. */
export const PreviewPanel: React.FC<PreviewPanelProps> = ({
	workspace,
	agent,
	tab,
}) => {
	const { isActive, isPanelOpen } = useTabPanelState();
	// Sign in once the tab is first shown, so restored tabs do not each sign
	// in on page load. The frame then stays mounted to keep its state.
	const [activated, setActivated] = useState(false);
	if (isActive && isPanelOpen && !activated) {
		setActivated(true);
	}

	if (agent.status !== "connected") {
		return (
			<div className="flex h-full items-center justify-center px-6 text-center text-xs text-content-secondary">
				The preview will be available once the workspace agent reconnects.
			</div>
		);
	}
	if (!activated) {
		return null;
	}

	const openSearch = new URLSearchParams({
		owner: workspace.owner_name,
		workspace: workspace.name,
		agent: tab.agentName,
		slug: tab.slug,
		...(tab.urlPath && { path: tab.urlPath }),
	});

	return (
		<PreviewFrame
			preview={{
				workspace_id: tab.workspaceId,
				agent_name: tab.agentName,
				preview_slug: tab.slug,
				url_path: tab.urlPath,
			}}
			title={tab.label}
		>
			<Button size="icon" variant="subtle" asChild>
				<Link
					to={`/previews/open?${openSearch}`}
					target="_blank"
					rel="noreferrer"
					aria-label="Open preview in new tab"
				>
					<ExternalLinkIcon />
				</Link>
			</Button>
		</PreviewFrame>
	);
};
