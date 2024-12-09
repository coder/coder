import DownloadOutlined from "@mui/icons-material/DownloadOutlined";
import Button from "@mui/material/Button";
import type {
	GroupSyncSettings,
	Organization,
	RoleSyncSettings,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { saveAs } from "file-saver";
import { type FC, useState } from "react";

interface DownloadPolicyButtonProps {
	syncSettings: RoleSyncSettings | GroupSyncSettings | undefined;
	type: "groups" | "roles";
	organization: Organization;
	download?: (file: Blob, filename: string) => void;
}

export const ExportPolicyButton: FC<DownloadPolicyButtonProps> = ({
	syncSettings,
	type,
	organization,
	download = saveAs,
}) => {
	const [isDownloading, setIsDownloading] = useState(false);

	const canCreatePolicyJson =
		syncSettings?.field && Object.keys(syncSettings?.mapping).length > 0;

	return (
		<Button
			startIcon={<DownloadOutlined />}
			disabled={!canCreatePolicyJson || isDownloading}
			onClick={async () => {
				if (canCreatePolicyJson) {
					try {
						setIsDownloading(true);
						const file = new Blob([JSON.stringify(syncSettings, null, 2)], {
							type: "application/json",
						});
						download(file, `${organization.name}_${type}-policy.json`);
					} catch (e) {
						console.error(e);
						displayError("Failed to export policy json");
					} finally {
						setIsDownloading(false);
					}
				}
			}}
		>
			Export Policy
		</Button>
	);
};
