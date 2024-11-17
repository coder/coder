import type { OrganizationSyncSettings } from "api/typesGenerated";
import { displayError } from "components/GlobalSnackbar/utils";
import { Button } from "components/ui/button";
import { saveAs } from "file-saver";
import { Download } from "lucide-react";
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
		return syncSettings?.field && Object.keys(syncSettings.mapping).length > 0
			? JSON.stringify(syncSettings, null, 2)
			: null;
	}, [syncSettings]);

	return (
		<Button
			variant={"outline"}
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
			<Download size={14} />
			Export Policy
		</Button>
	);
};
