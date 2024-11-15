import DownloadOutlined from "@mui/icons-material/DownloadOutlined";
import Button from "@mui/material/Button";
import type { OrganizationSyncSettings } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { saveAs } from "file-saver";
import { type FC, useMemo, useState } from "react";

interface ExportPolicyButtonProps {
	syncSettings: OrganizationSyncSettings | undefined;
	download?: (file: Blob, filename: string) => void;
}

export const ExportPolicyButton: FC<ExportPolicyButtonProps> = ({
	syncSettings,
	download = saveAs,
}) => {
	const [isDownloading, setIsDownloading] = useState(false);

	const policyJSON = useMemo(() => {
		return syncSettings?.field && syncSettings.mapping
			? JSON.stringify(syncSettings, null, 2)
			: null;
	}, [syncSettings]);
	console.log({ syncSettings });
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
						download(file, "organizations_policy.json");
					} catch (e) {
						console.error(e);
						displayError("Failed to export organizations policy json");
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
