import DownloadOutlined from "@mui/icons-material/DownloadOutlined";
import Button from "@mui/material/Button";
import type { Organization } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { saveAs } from "file-saver";
import { type FC, useState } from "react";

interface DownloadPolicyButtonProps {
	policy: string | null;
	type: "groups" | "roles";
	organization: Organization;
}

export const ExportPolicyButton: FC<DownloadPolicyButtonProps> = ({
	policy,
	type,
	organization,
}) => {
	const [isDownloading, setIsDownloading] = useState(false);

	return (
		<Button
			startIcon={<DownloadOutlined />}
			disabled={!policy || isDownloading}
			onClick={async () => {
				if (policy) {
					try {
						setIsDownloading(true);
						const file = new Blob([policy], {
							type: "application/json",
						});
						saveAs(file, `${organization.name}_${type}-policy.json`);
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
