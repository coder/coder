import type { OrganizationSyncSettings } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { saveAs } from "file-saver";
import { Download } from "lucide-react";
import { type FC, useState } from "react";

interface ExportPolicyButtonProps {
	syncSettings: OrganizationSyncSettings | undefined;
	download?: (file: Blob, filename: string) => void;
}

export const ExportPolicyButton: FC<ExportPolicyButtonProps> = ({
	syncSettings,
	download = saveAs,
}) => {
	const [isDownloading, setIsDownloading] = useState(false);

	const canCreatePolicyJson =
		syncSettings?.field && Object.keys(syncSettings?.mapping).length > 0;

	return (
		<Button
			variant={"outline"}
			disabled={!canCreatePolicyJson || isDownloading}
			onClick={async () => {
				if (canCreatePolicyJson) {
					try {
						setIsDownloading(true);
						const file = new Blob([JSON.stringify(syncSettings, null, 2)], {
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
			<Download size={14} />
			Export Policy
		</Button>
	);
};
