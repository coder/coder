import { saveAs } from "file-saver";
import { ChevronDownIcon, DownloadIcon, PackageIcon } from "lucide-react";
import { type FC, type ReactNode, useState } from "react";
import { toast } from "sonner";
import { getErrorDetail } from "#/api/errors";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";

type DownloadableLogSet = {
	label: string;
	filenameSuffix: string;
	logsText: string;
	startIcon?: ReactNode;
};

type DownloadSelectedAgentLogsButtonProps = {
	agentName: string;
	logSets: readonly DownloadableLogSet[];
	allLogsText: string;
	disabled?: boolean;
	download?: (file: Blob, filename: string) => void | Promise<void>;
};

export const DownloadSelectedAgentLogsButton: FC<
	DownloadSelectedAgentLogsButtonProps
> = ({
	agentName,
	logSets,
	allLogsText,
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

	const downloadLogs = (logsText: string, filenameSuffix: string) => {
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
	};

	const hasAllLogs = allLogsText.length > 0;

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
