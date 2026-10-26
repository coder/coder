import { ExternalLinkIcon, RefreshCwIcon } from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
import { checkAuthorization } from "#/api/queries/authCheck";
import { chatPreviews } from "#/api/queries/chats";
import type { Chat, Workspace } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { workspaceAppURL } from "#/utils/portForward";
import { getWorkspaceAgents } from "#/utils/workspace";
import {
	getPersistedSelectedPreview,
	savePersistedSelectedPreview,
} from "../../utils/rightPanelTabStorage";
import { previewKey } from "../ChatElements/tools/PreviewPanelContext";
import {
	DesktopWorkspaceState,
	isDesktopReachable,
	useStartDesktopWorkspace,
} from "./DesktopWorkspaceState";

type PreviewPanelProps = {
	chat: Chat;
	workspace: Workspace;
	host: string;
	/** When true the panel is the active sidebar tab. */
	isVisible: boolean;
};

/** Lists the chat's previews and displays the selected one. */
export const PreviewPanel: React.FC<PreviewPanelProps> = ({
	chat,
	workspace,
	host,
	isVisible,
}) => {
	// Load the preview page once the tab is first shown, then keep it
	// mounted so switching tabs preserves its state.
	const [activated, setActivated] = useState(false);
	if (isVisible && !activated) {
		setActivated(true);
	}

	const previewsQuery = useQuery(chatPreviews(chat.id, workspace.id));
	const previews = previewsQuery.data ?? [];

	// Previews are workspace apps, so viewers of a shared chat can only open
	// them if they can also connect to the workspace's apps.
	const accessQuery = useQuery(
		checkAuthorization({
			checks: {
				canOpenPreviews: {
					object: {
						resource_type: "workspace",
						resource_id: workspace.id,
						owner_id: workspace.owner_id,
					},
					action: "application_connect",
				},
			},
		}),
	);
	const canOpenPreviews = accessQuery.data?.canOpenPreviews === true;
	const error = previewsQuery.error ?? accessQuery.error;

	const [selectedKey, setSelectedKey] = useState(() =>
		getPersistedSelectedPreview(chat.id),
	);
	const selected =
		selectedKey === null
			? previews[0]
			: previews.find((preview) => previewKey(preview) === selectedKey);

	const selectPreview = (key: string) => {
		setSelectedKey(key);
		if (!chat.archived) {
			savePersistedSelectedPreview(chat.id, key);
		}
	};

	const [reloadCount, setReloadCount] = useState(0);
	const { startWorkspace, isStartingWorkspace } =
		useStartDesktopWorkspace(workspace);

	const viewable = canOpenPreviews ? selected : undefined;
	// The app proxy serves an error page until the preview's agent connects.
	const previewAgent = getWorkspaceAgents(workspace).find(
		(agent) => agent.name === viewable?.agent_name,
	);
	const isPreviewReachable = isDesktopReachable(
		workspace.latest_build.status,
		previewAgent?.status,
	);

	let url: string | undefined;
	if (viewable) {
		// The server only lists a path and query, so the host cannot change.
		const urlPath = viewable.url_path ?? "";
		const queryStart = urlPath.indexOf("?");
		const pathname = queryStart === -1 ? urlPath : urlPath.slice(0, queryStart);
		const search = queryStart === -1 ? undefined : urlPath.slice(queryStart);
		const appURL = workspaceAppURL(
			host,
			`preview--${viewable.preview_id || viewable.port}`,
			viewable.agent_name,
			workspace.name,
			workspace.owner_name,
			pathname || undefined,
			search,
		);
		// An unusable wildcard hostname produces a do-nothing "#" link.
		url = appURL === "#" ? undefined : appURL;
	}

	let message: React.ReactNode;
	if (previewsQuery.isLoading || accessQuery.isLoading) {
		message = <Spinner loading />;
	} else if (accessQuery.data?.canOpenPreviews === false) {
		message = "You don't have access to this workspace's previews.";
	} else if (!selected && previews.length > 0) {
		message = "Select a preview.";
	} else if (!selected && !error) {
		message = "No previews yet.";
	} else if (viewable && isPreviewReachable && !url) {
		message = "Previews require a wildcard access URL.";
	}

	return (
		<div className="flex h-full min-h-0 flex-col">
			<div className="flex h-8 shrink-0 items-center gap-1 border-0 border-b border-solid border-border bg-surface-primary px-4 lg:px-3">
				<Select
					value={selected ? previewKey(selected) : ""}
					onValueChange={selectPreview}
					disabled={previews.length === 0}
				>
					<SelectTrigger
						aria-label="Preview"
						className="h-6 w-auto min-w-0 max-w-full gap-2 px-2 py-0 text-xs text-content-primary shadow-none"
					>
						<SelectValue placeholder="Select a preview" />
					</SelectTrigger>
					<SelectContent>
						{previews.map((preview) => (
							<SelectItem
								key={previewKey(preview)}
								value={previewKey(preview)}
								className="text-xs"
							>
								{preview.title}
								<span className="ml-2 text-content-secondary">
									{preview.path
										? "files"
										: `port ${preview.port}${preview.url_path ?? ""}`}
								</span>
							</SelectItem>
						))}
					</SelectContent>
				</Select>
				<div className="flex-1" />
				<Button
					size="icon"
					variant="subtle"
					className="size-7 p-0"
					aria-label="Refresh preview"
					disabled={previewsQuery.isFetching}
					onClick={() => {
						setReloadCount((current) => current + 1);
						void previewsQuery.refetch();
					}}
				>
					<RefreshCwIcon />
				</Button>
				{isPreviewReachable && url && (
					<Button size="icon" variant="subtle" className="size-7 p-0" asChild>
						<a
							href={url}
							target="_blank"
							rel="noreferrer"
							aria-label="Open preview in new tab"
						>
							<ExternalLinkIcon />
						</a>
					</Button>
				)}
			</div>
			{error && (
				<ErrorAlert
					error={error}
					showDebugDetail={false}
					className="shrink-0 rounded-none border-0 border-b"
					actions={
						<Button
							size="sm"
							variant="outline"
							onClick={() => {
								void previewsQuery.refetch();
								void accessQuery.refetch();
							}}
						>
							Retry
						</Button>
					}
				/>
			)}
			{message && (
				<div className="flex min-h-0 flex-1 items-center justify-center px-6 text-center text-xs text-content-secondary">
					{message}
				</div>
			)}
			{viewable && !isPreviewReachable && (
				<div className="min-h-0 flex-1">
					<DesktopWorkspaceState
						workspace={workspace}
						onStartWorkspace={startWorkspace}
						isStartingWorkspace={isStartingWorkspace}
						subject="preview"
					/>
				</div>
			)}
			{activated && viewable && isPreviewReachable && url && (
				// Preview pages serve agent-authored content. These flags match
				// the sandbox coderd sets on preview responses: the page keeps
				// its origin but cannot navigate the dashboard or open popups.
				<iframe
					key={reloadCount}
					src={url}
					title={viewable.title}
					sandbox="allow-scripts allow-same-origin allow-forms allow-modals allow-downloads"
					referrerPolicy="no-referrer"
					className="size-full border-0"
				/>
			)}
		</div>
	);
};
