import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";

interface DataProtectionBannerProps {
	dataProtectionEnabled: boolean | undefined;
}

export const DataProtectionBanner: FC<DataProtectionBannerProps> = ({
	dataProtectionEnabled,
}) => {
	if (!dataProtectionEnabled) {
		return null;
	}
	return (
		<Alert severity="info">
			Data Protection Mode is active. Individual user identifiers are obfuscated
			in reports to comply with employee data protection regulations.
		</Alert>
	);
};
