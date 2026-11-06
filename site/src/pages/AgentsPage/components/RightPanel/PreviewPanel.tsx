import { ExternalLinkIcon, RefreshCwIcon } from "lucide-react";
import { useState } from "react";
import { useQuery } from "react-query";
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
import { getWorkspaceAgents } from "#/utils/workspace";
import {
	getPersistedSelectedPreview,
	savePersistedSelectedPreview,
} from "../../utils/rightPanelTabStorage";
import { useTabPanelState } from "../ChatsSidebar/tabs/SidebarTabView";
import { PreviewFrame, previewKey, previewURL } from "./PreviewFrame";

type PreviewPanelProps = {
	chat: Chat;
	workspace: Workspace;
	host: string;
};

export const PreviewPanel: React.FC<PreviewPanelProps> = ({
	chat,
	workspace,
	host,
}) => {
	const { isActive, isPanelOpen } = useTabPanelState();

	// Load the preview page once the tab is first shown, then keep it
	// mounted so switching tabs preserves its state.
	const [activated, setActivated] = useState(false);
	if (isActive && isPanelOpen && !activated) {
		setActivated(true);
	}

	const previewsQuery = useQuery(chatPreviews(chat.id, workspace.id));
	const previews = previewsQuery.data ?? [];

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

	// The app proxy serves an error page until the preview's agent connects.
	const isAgentConnected =
		getWorkspaceAgents(workspace).find(
			(agent) => agent.name === selected?.agent_name,
		)?.status === "connected";

	const url = selected && previewURL(selected, workspace, host);

	let message: React.ReactNode;
	// A newly opened preview is missing from the cached list until the refetch.
	if (previewsQuery.isLoading || (!selected && previewsQuery.isFetching)) {
		message = <Spinner loading label="Loading previews" />;
	} else if (!selected && previews.length > 0) {
		message = "Select a preview.";
	} else if (!selected && !previewsQuery.error) {
		message = "No previews yet.";
	} else if (selected && !isAgentConnected) {
		message =
			"The preview will be available once the workspace agent connects.";
	} else if (selected && !url) {
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
					onClick={() => {
						setReloadCount((current) => current + 1);
						void previewsQuery.refetch();
					}}
				>
					<RefreshCwIcon />
				</Button>
				{isAgentConnected && selected && url && (
					<Button size="icon" variant="subtle" className="size-7 p-0" asChild>
						<a
							href={`/agents/${chat.id}/preview?${new URLSearchParams({ key: previewKey(selected) })}`}
							target="_blank"
							rel="noreferrer"
							aria-label="Open preview in new tab"
						>
							<ExternalLinkIcon />
						</a>
					</Button>
				)}
			</div>
			{previewsQuery.error && (
				<ErrorAlert
					error={previewsQuery.error}
					showDebugDetail={false}
					className="shrink-0 rounded-none border-0 border-b"
				/>
			)}
			{message && (
				<div className="flex min-h-0 flex-1 items-center justify-center px-6 text-center text-xs text-content-secondary">
					{message}
				</div>
			)}
			{activated && selected && isAgentConnected && url && (
				<PreviewFrame key={reloadCount} url={url} title={selected.title} />
			)}
		</div>
	);
};
