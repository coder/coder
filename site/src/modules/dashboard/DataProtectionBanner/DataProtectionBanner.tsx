import { useQuery, useMutation, useQueryClient } from "react-query";
import type { FC } from "react";
import { dataProtectionStatus } from "#/api/queries/deployment";
import {
	preferenceSettings,
	preferencesQueryKey,
	updatePreferenceSettings,
} from "#/api/queries/users";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import { XIcon } from "lucide-react";

export const DataProtectionBanner: FC = () => {
	const dpStatus = useQuery(dataProtectionStatus());
	const prefs = useQuery(preferenceSettings());
	const queryClient = useQueryClient();
	const updatePrefs = useMutation(updatePreferenceSettings(queryClient));

	if (!dpStatus.data?.enabled) {
		return null;
	}

	if (prefs.data?.dpm_banner_hidden) {
		return null;
	}

	const tier = dpStatus.data.tier ?? 1;
	const isAuditor = dpStatus.data.auditor;
	const infoURL = dpStatus.data.info_url;

	const handleDismiss = () => {
		const current =
			queryClient.getQueryData<UserPreferenceSettings>(preferencesQueryKey);
		if (current) {
			queryClient.setQueryData<UserPreferenceSettings>(preferencesQueryKey, {
				...current,
				dpm_banner_hidden: true,
			});
		}
		updatePrefs.mutate({ dpm_banner_hidden: true });
	};

	let message: string;
	if (isAuditor) {
		message = `Data Protection Mode (Tier ${tier}) is active. As a designated auditor, your access to unobfuscated data is logged.`;
	} else if (tier >= 2) {
		message =
			"Data Protection Mode is active. User-identifying data is not stored in analytics and audit records.";
	} else {
		message =
			"Data Protection Mode is active. Individual user identifiers are obfuscated to comply with data protection regulations.";
	}

	return (
		<div
			role="status"
			className="flex items-center justify-between gap-2 bg-surface-secondary px-4 py-1.5 text-xs text-content-secondary border-b border-solid border-border"
		>
			<div className="flex items-center gap-1.5">
				<span className="font-medium">🔒</span>
				<span>{message}</span>
				{infoURL && (
					<a
						href={infoURL}
						target="_blank"
						rel="noopener noreferrer"
						className="text-content-link hover:underline ml-1"
					>
						Learn more
					</a>
				)}
			</div>
			<button
				type="button"
				onClick={handleDismiss}
				className="flex items-center justify-center p-0.5 rounded hover:bg-surface-tertiary text-content-secondary hover:text-content-primary transition-colors"
				aria-label="Dismiss Data Protection Mode banner"
			>
				<XIcon className="size-3.5" />
			</button>
		</div>
	);
};
