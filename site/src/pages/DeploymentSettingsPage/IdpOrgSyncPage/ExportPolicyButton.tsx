import { getErrorDetail } from "api/errors";
import type { OrganizationSyncSettings } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { saveAs } from "file-saver";
import { Download } from "lucide-react";
import { type FC, useState } from "react";
import { toast } from "sonner";

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
			variant="outline"
			disabled={!canCreatePolicyJson || isDownloading}
			onClick={async () => {
				if (canCreatePolicyJson) {
					try {
						setIsDownloading(true);
						const file = new Blob([JSON.stringify(syncSettings, null, 2)], {
							type: "application/json",
						});
						download(file, "organizations_policy.json");
					} catch (error) {
						console.error(error);
						toast.error("Failed to export organizations policy JSON.", {
							description: getErrorDetail(error),
						});
					} finally {
						setIsDownloading(false);
					}
				}
			}}
		>
			<Download />
			Export Policy
		</Button>
	);
};
