import { agentLogs } from "api/queries/workspaces";
import type { WorkspaceAgent, WorkspaceAgentLog } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { saveAs } from "file-saver";
import { DownloadIcon } from "lucide-react";
import { type FC, useState } from "react";
import { useQueryClient } from "react-query";

type DownloadAgentLogsButtonProps = {
	agent: Pick<WorkspaceAgent, "id" | "name" | "status" | "lifecycle_state">;
	download?: (file: Blob, filename: string) => void;
};

export const DownloadAgentLogsButton: FC<DownloadAgentLogsButtonProps> = ({
	agent,
	download = saveAs,
}) => {
	const queryClient = useQueryClient();
	const isConnected = agent.status === "connected";
	const [isDownloading, setIsDownloading] = useState(false);

	const fetchLogs = async () => {
		const queryOpts = agentLogs(agent.id);
		let logs = queryClient.getQueryData<WorkspaceAgentLog[]>(
			queryOpts.queryKey,
		);
		if (!logs) {
			logs = await queryClient.fetchQuery(queryOpts);
		}
		return logs;
	};

	return (
		<Button
			disabled={!isConnected || isDownloading}
			variant="subtle"
			size="sm"
			onClick={async () => {
				try {
					setIsDownloading(true);
					const logs = await fetchLogs();
					if (!logs) {
						throw new Error("No logs found");
					}
					const text = logs.map((l) => l.output).join("\n");
					const file = new Blob([text], { type: "text/plain" });
					download(file, `${agent.name}-logs.txt`);
				} catch (e) {
					console.error(e);
					displayError("Failed to download logs");
				} finally {
					setIsDownloading(false);
				}
			}}
		>
			<DownloadIcon />
			{isDownloading ? "Downloading..." : "Download logs"}
		</Button>
	);
};
