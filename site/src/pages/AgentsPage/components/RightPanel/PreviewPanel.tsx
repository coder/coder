import { useState } from "react";
import type { WorkspaceAgent } from "#/api/typesGenerated";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";
import { useTabPanelState } from "../ChatsSidebar/tabs/SidebarTabView";
import { PreviewFrame } from "./PreviewFrame";

type PreviewPanelProps = {
	agent: WorkspaceAgent;
	tab: Extract<UserRightPanelTab, { kind: "preview" }>;
	/** Changing it loads the preview again. */
	reloadCount: number;
};

/** Shows a port or file preview that the agent opened with `open_preview`. */
export const PreviewPanel: React.FC<PreviewPanelProps> = ({
	agent,
	tab,
	reloadCount,
}) => {
	const { isActive, isPanelOpen } = useTabPanelState();
	// Load the preview once the tab is first shown, so restored tabs do not
	// each load on page load. The frame then stays mounted to keep its state.
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

	return (
		<PreviewFrame
			key={reloadCount}
			preview={{
				workspace_id: tab.workspaceId,
				agent_name: tab.agentName,
				preview_slug: tab.slug,
				url_path: tab.urlPath,
			}}
			title={tab.label}
		/>
	);
};
