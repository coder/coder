import { ExternalLinkIcon, NetworkIcon } from "lucide-react";
import type { Workspace, WorkspaceAgent } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { WorkspaceIframe } from "#/modules/apps/WorkspaceAppFrame";
import { filePreviewURL, portForwardURL } from "#/utils/portForward";
import { splitURLPath } from "#/utils/uri";
import type { UserRightPanelTab } from "../../utils/rightPanelTabs";

/** Shows a forwarded port or a file preview that the workspace agent serves. */
export const WorkspacePreviewPanel: React.FC<{
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	tab: Extract<UserRightPanelTab, { kind: "port" | "file_preview" }>;
}> = ({ workspace, agent, host, tab }) => {
	const url = getPreviewURL({ workspace, agent, host, tab });
	const openLabel =
		tab.kind === "port" ? "Open port in new tab" : "Open preview in new tab";
	const unavailableMessage = getUnavailableMessage({
		host,
		agent,
		url,
		kindLabel: tab.kind === "port" ? "Port" : "File",
	});

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex shrink-0 items-center gap-2 border-b bg-surface-secondary px-2 py-1 text-xs text-content-secondary">
				<NetworkIcon className="size-3.5 shrink-0" />
				<span className="min-w-0 truncate text-content-primary">
					{tab.label}
				</span>
				<div className="flex-1" />
				{unavailableMessage ? (
					<Button size="icon" variant="subtle" disabled aria-label={openLabel}>
						<ExternalLinkIcon />
					</Button>
				) : (
					<Button size="icon" variant="subtle" asChild>
						<a
							href={url}
							target="_blank"
							rel="noreferrer"
							aria-label={openLabel}
						>
							<ExternalLinkIcon />
						</a>
					</Button>
				)}
			</div>
			{unavailableMessage ? (
				<div className="flex min-h-0 flex-1 items-center justify-center px-6 text-center text-xs text-content-secondary">
					{unavailableMessage}
				</div>
			) : (
				<WorkspaceIframe src={url} title={tab.label} />
			)}
		</div>
	);
};

function getPreviewURL({
	workspace,
	agent,
	host,
	tab,
}: {
	workspace: Workspace;
	agent: WorkspaceAgent;
	host: string;
	tab: Extract<UserRightPanelTab, { kind: "port" | "file_preview" }>;
}): string {
	const { pathname, search } = splitURLPath(tab.urlPath ?? "");
	switch (tab.kind) {
		case "port":
			return portForwardURL(
				host,
				tab.port,
				agent.name,
				workspace.name,
				workspace.owner_name,
				tab.protocol,
				pathname,
				search,
			);
		case "file_preview":
			return filePreviewURL(
				host,
				tab.previewId,
				agent.name,
				workspace.name,
				workspace.owner_name,
				pathname,
				search,
			);
		default: {
			const _exhaustive: never = tab;
			return _exhaustive;
		}
	}
}

function getUnavailableMessage({
	host,
	agent,
	url,
	kindLabel,
}: {
	host: string;
	agent: WorkspaceAgent;
	url: string;
	kindLabel: string;
}): string | undefined {
	if (host.trim() === "") {
		return `${kindLabel} previews require a wildcard access URL.`;
	}
	if (agent.status !== "connected") {
		return `${kindLabel} preview will be available once the workspace agent reconnects.`;
	}
	if (url === "#") {
		return "The wildcard access URL produced an invalid preview URL. Check the deployment's wildcard access URL configuration.";
	}
	return undefined;
}
