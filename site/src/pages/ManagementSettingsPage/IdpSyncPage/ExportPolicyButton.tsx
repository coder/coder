import DownloadOutlined from "@mui/icons-material/DownloadOutlined";
import Button from "@mui/material/Button";
import type {
	GroupSyncSettings,
	Organization,
	RoleSyncSettings,
} from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { saveAs } from "file-saver";
import { type FC, useMemo, useState } from "react";

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

	const policyJSON = useMemo(() => {
		return syncSettings?.field && syncSettings.mapping
			? JSON.stringify(syncSettings, null, 2)
			: null;
	}, [syncSettings]);

	return (
		<Button
			startIcon={<DownloadOutlined />}
			disabled={!policyJSON || isDownloading}
			onClick={async () => {
				if (policyJSON) {
					try {
						setIsDownloading(true);
						const file = new Blob([policyJSON], {
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
