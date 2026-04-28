import type { FC } from "react";
import { Alert } from "#/components/Alert/Alert";

interface DataProtectionBannerProps {
	dataProtectionEnabled: boolean | undefined;
	tier?: number;
	isAuditor?: boolean;
}

export const DataProtectionBanner: FC<DataProtectionBannerProps> = ({
	dataProtectionEnabled,
	tier,
	isAuditor,
}) => {
	if (!dataProtectionEnabled) {
		return null;
	}

	const tierLevel = tier ?? 1;

	if (isAuditor) {
		return (
			<Alert severity="warning" css={{ marginBottom: 16 }}>
				Data Protection Mode (Tier {tierLevel}) is active. As a designated
				auditor, you have access to unobfuscated user data. All access is logged
				for compliance purposes.
			</Alert>
		);
	}

	const description =
		tierLevel >= 2
			? "Individual user identifiers are obfuscated across all interfaces to comply with employee data protection regulations."
			: "Individual user identifiers are obfuscated in reports to comply with employee data protection regulations.";

	return (
		<Alert severity="info" css={{ marginBottom: 16 }}>
			Data Protection Mode (Tier {tierLevel}) is active. {description}
		</Alert>
	);
};
