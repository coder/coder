import { saveAs } from "file-saver";
import { DownloadIcon } from "lucide-react";
import { type FC, useState } from "react";
import { toast } from "sonner";
import { getErrorDetail } from "#/api/errors";
import type {
	GroupSyncSettings,
	OrganizationSyncSettings,
	RoleSyncSettings,
} from "#/api/typesGenerated";
import { Button, type ButtonProps } from "#/components/Button/Button";

type ExportableSyncSettings =
	| GroupSyncSettings
	| RoleSyncSettings
	| OrganizationSyncSettings;

interface ExportPolicyButtonProps {
	syncSettings: ExportableSyncSettings | undefined;
	filename: string;
	size?: ButtonProps["size"];
	download?: (file: Blob, filename: string) => void;
}

export const ExportPolicyButton: FC<ExportPolicyButtonProps> = ({
	syncSettings,
	filename,
	size = "sm",
	download = saveAs,
}) => {
	const [isDownloading, setIsDownloading] = useState(false);

	const canCreatePolicyJson =
		Boolean(syncSettings?.field) &&
		Object.keys(syncSettings?.mapping ?? {}).length > 0;

	return (
		<Button
			size={size}
			variant="outline"
			disabled={!canCreatePolicyJson || isDownloading}
			onClick={async () => {
				if (!canCreatePolicyJson) {
					return;
				}

				try {
					setIsDownloading(true);
					const file = new Blob([JSON.stringify(syncSettings, null, 2)], {
						type: "application/json",
					});
					download(file, filename);
				} catch (error) {
					toast.error("Failed to export policy JSON.", {
						description: getErrorDetail(error),
					});
				} finally {
					setIsDownloading(false);
				}
			}}
		>
			<DownloadIcon />
			Export policy
		</Button>
	);
};
