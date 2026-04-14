import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";

interface DataProtectionBannerProps {
	dataProtectionEnabled: boolean | undefined;
	isAuditor?: boolean;
}

export const DataProtectionBanner: FC<DataProtectionBannerProps> = ({
	dataProtectionEnabled,
	isAuditor,
}) => {
	if (!dataProtectionEnabled) {
		return null;
	}
	if (isAuditor) {
		return (
			<Alert severity="warning" css={{ marginBottom: 16 }}>
				Data Protection Mode is active. As a designated auditor, you have access
				to unobfuscated user data. All access is logged for compliance purposes.
			</Alert>
		);
	}
	return (
		<Alert severity="info" css={{ marginBottom: 16 }}>
			Data Protection Mode is active. Individual user identifiers are obfuscated
			in reports to comply with employee data protection regulations.
		</Alert>
	);
};
