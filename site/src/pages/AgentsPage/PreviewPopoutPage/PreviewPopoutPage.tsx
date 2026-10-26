import { useQuery } from "react-query";
import { useParams, useSearchParams } from "react-router";
import { chat, chatPreviews } from "#/api/queries/chats";
import { workspaceById } from "#/api/queries/workspaces";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Spinner } from "#/components/Spinner/Spinner";
import { useProxy } from "#/contexts/ProxyContext";
import {
	PreviewFrame,
	previewKey,
	previewURL,
} from "../components/RightPanel/PreviewFrame";

export default function PreviewPopoutPage() {
	const { agentId } = useParams() as { agentId: string };
	const [searchParams] = useSearchParams();
	const { proxy } = useProxy();

	const chatQuery = useQuery(chat(agentId));
	const workspaceId = chatQuery.data?.workspace_id ?? "";
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId),
		enabled: workspaceId !== "",
	});
	const previewsQuery = useQuery({
		...chatPreviews(agentId, workspaceId),
		enabled: workspaceId !== "",
	});

	const workspace = workspaceQuery.data;
	const preview = previewsQuery.data?.find(
		(preview) => previewKey(preview) === searchParams.get("key"),
	);
	const url =
		workspace &&
		preview &&
		previewURL(preview, workspace, proxy.preferredWildcardHostname);
	const error = chatQuery.error ?? workspaceQuery.error ?? previewsQuery.error;

	if (preview && url) {
		// The address bar shows the dashboard, so say whose page this is.
		return (
			<div className="flex h-screen w-screen flex-col">
				<div className="flex h-8 shrink-0 items-center gap-2 border-0 border-b border-solid border-border bg-surface-primary px-3 text-xs">
					<span className="truncate font-medium text-content-primary">
						{preview.title}
					</span>
					<span className="truncate text-content-secondary">
						{new URL(url).host}
					</span>
					<span className="ml-auto shrink-0 text-content-secondary">
						Opened by the agent
					</span>
				</div>
				<div className="min-h-0 flex-1">
					<PreviewFrame url={url} title={preview.title} />
				</div>
			</div>
		);
	}

	return (
		<div className="flex h-screen w-screen items-center justify-center bg-surface-primary p-6 text-sm text-content-secondary">
			{error ? (
				<ErrorAlert error={error} />
			) : chatQuery.isLoading ||
				workspaceQuery.isLoading ||
				previewsQuery.isLoading ? (
				<Spinner loading label="Loading preview" />
			) : (
				"This preview is not available."
			)}
		</div>
	);
}
