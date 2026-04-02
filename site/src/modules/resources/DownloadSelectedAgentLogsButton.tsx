import { saveAs } from "file-saver";
import { DownloadIcon } from "lucide-react";
import { type FC, useState } from "react";
import { toast } from "sonner";
import { getErrorDetail } from "#/api/errors";
import { Button } from "#/components/Button/Button";

type DownloadSelectedAgentLogsButtonProps = {
	agentName: string;
	filenameSuffix: string;
	logsText: string;
	disabled?: boolean;
	download?: (file: Blob, filename: string) => void;
};

export const DownloadSelectedAgentLogsButton: FC<
	DownloadSelectedAgentLogsButtonProps
> = ({
	agentName,
	filenameSuffix,
	logsText,
	disabled = false,
	download = saveAs,
}) => {
	const [isDownloading, setIsDownloading] = useState(false);

	return (
		<Button
			variant="subtle"
			size="sm"
			disabled={disabled || isDownloading}
			onClick={() => {
				try {
					setIsDownloading(true);
					const file = new Blob([logsText], { type: "text/plain" });
					download(file, `${agentName}-${filenameSuffix}.txt`);
				} catch (error) {
					console.error(error);
					toast.error(`Failed to download "${agentName}" logs.`, {
						description: getErrorDetail(error),
					});
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
