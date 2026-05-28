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
		const auditorDetail =
			tierLevel >= 2
				? " New user-identifying data is no longer being stored. Historical entries from before this mode was enabled may still appear."
				: "";
		return (
			<Alert severity="warning" css={{ marginBottom: 16 }}>
				Data Protection Mode (Tier {tierLevel}) is active. As a designated
				auditor, you have access to unobfuscated user data. All access is
				logged for compliance purposes.{auditorDetail}
			</Alert>
		);
	}

	if (tierLevel >= 2) {
		return (
			<Alert severity="info" css={{ marginBottom: 16 }}>
				Data Protection Mode (Tier {tierLevel}) is active. New
				user-identifying data is no longer being stored. Historical entries
				from before this mode was enabled may still appear with obfuscated
				identities and will be automatically removed according to your data
				retention policy.
			</Alert>
		);
	}

	return (
		<Alert severity="info" css={{ marginBottom: 16 }}>
			Data Protection Mode (Tier {tierLevel}) is active. Individual user
			identifiers are obfuscated across all interfaces to comply with
			employee data protection regulations.
		</Alert>
	);
};
