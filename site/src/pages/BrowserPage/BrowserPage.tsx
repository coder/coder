import { useQuery } from "react-query";
import { useParams } from "react-router";
import { workspaceByOwnerAndName } from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Loader } from "#/components/Loader/Loader";
import { BrowserView } from "#/modules/browser/BrowserView";
import { pageTitle } from "#/utils/page";
import { getMatchingAgentOrFirst } from "#/utils/workspace";

const browserStreamUrl = (agentId: string): string => {
	const protocol = location.protocol === "https:" ? "wss:" : "ws:";
	return `${protocol}//${location.host}/api/v2/workspaceagents/${agentId}/browser`;
};

const BrowserPage: React.FC = () => {
	const { username = "", workspace: workspaceParam = "" } = useParams();
	// The workspace segment is <workspace name>[.<agent name>].
	const [workspaceName, agentName] = workspaceParam.split(".");
	const workspace = useQuery(
		workspaceByOwnerAndName(username.replace("@", ""), workspaceName),
	);
	const agent = workspace.data
		? getMatchingAgentOrFirst(workspace.data, agentName)
		: undefined;
	if (agent) {
		return (
			<div className="h-screen w-screen overflow-hidden">
				<BrowserView
					streamUrl={browserStreamUrl(agent.id)}
					workspaceName={workspaceName}
				/>
			</div>
		);
	}

	return (
		<>
			<title>{pageTitle("Browser", workspaceName)}</title>
			{workspace.error ? (
				<div className="p-6">
					<ErrorAlert error={workspace.error} />
				</div>
			) : workspace.data ? (
				<div className="p-6">
					<ErrorAlert error="This workspace has no agent. Is it running?" />
				</div>
			) : (
				<Loader fullscreen />
			)}
		</>
	);
};

export default BrowserPage;
