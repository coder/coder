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
	download?: (file: Blob, filename: string) => void | Promise<void>;
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
	const handleDownload = async () => {
		try {
			setIsDownloading(true);
			const file = new Blob([logsText], { type: "text/plain" });
			await download(file, `${agentName}-${filenameSuffix}.txt`);
		} catch (error) {
			toast.error(`Failed to download "${agentName}" logs.`, {
				description: getErrorDetail(error),
			});
		} finally {
			setIsDownloading(false);
		}
	};

	return (
		<Button
			variant="subtle"
			size="sm"
			disabled={disabled || isDownloading}
			onClick={handleDownload}
		>
			<DownloadIcon />
			{isDownloading ? "Downloading..." : "Download logs"}
		</Button>
	);
};
