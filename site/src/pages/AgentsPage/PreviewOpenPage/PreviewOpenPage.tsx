import { useQuery } from "react-query";
import { useSearchParams } from "react-router";
import { workspaceByOwnerAndName } from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { pageTitle } from "#/utils/page";
import { PreviewFrame } from "../components/RightPanel/PreviewFrame";

/**
 * Shows one preview full-screen, for "Open in new tab" and for direct opens
 * of a preview address, which the server redirects here. The search params
 * name the preview: owner, workspace, agent, slug, and an optional path.
 */
export default function PreviewOpenPage() {
	const [searchParams] = useSearchParams();
	const owner = searchParams.get("owner") ?? "";
	const workspaceName = searchParams.get("workspace") ?? "";
	const agentName = searchParams.get("agent") ?? "";
	const slug = searchParams.get("slug") ?? "";
	const urlPath = searchParams.get("path") || undefined;
	const isLinkComplete =
		owner !== "" && workspaceName !== "" && agentName !== "" && slug !== "";

	const workspaceQuery = useQuery({
		...workspaceByOwnerAndName(owner, workspaceName),
		enabled: isLinkComplete,
	});
	const workspace = workspaceQuery.data;

	if (isLinkComplete && workspace) {
		// The address bar shows the dashboard, so the header says whose page
		// this is. It never shows the title the agent chose.
		return (
			<div className="flex h-screen w-screen flex-col bg-surface-primary">
				<title>{pageTitle("Preview")}</title>
				<PreviewFrame
					preview={{
						workspace_id: workspace.id,
						agent_name: agentName,
						preview_slug: slug,
						url_path: urlPath,
					}}
					title="Preview"
					headerContent={
						<span className="min-w-0 max-w-full truncate text-content-secondary">
							{agentName} in {workspace.owner_name}/{workspace.name}
						</span>
					}
				/>
			</div>
		);
	}

	return (
		<div className="flex h-screen w-screen items-center justify-center bg-surface-primary p-6 text-sm text-content-secondary">
			<title>{pageTitle("Preview")}</title>
			{!isLinkComplete ? (
				"This preview link is incomplete."
			) : workspaceQuery.error ? (
				<ErrorAlert error={workspaceQuery.error} />
			) : (
				<Spinner loading label="Loading preview" />
			)}
		</div>
	);
}
