import { XIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { dataProtectionStatus } from "#/api/queries/deployment";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";

export const DataProtectionBanner: FC = () => {
	const dpStatus = useQuery(dataProtectionStatus());
	const prefs = useQuery(preferenceSettings());
	const queryClient = useQueryClient();
	const updatePrefs = useMutation(updatePreferenceSettings(queryClient));

	const queryHidden = prefs.data?.dpm_banner_hidden ?? false;
	const [localDismissed, setLocalDismissed] = useState(false);

	// biome-ignore lint/correctness/useExhaustiveDependencies: reset local state when the server value changes
	useEffect(() => {
		setLocalDismissed(false);
	}, [queryHidden]);

	if (!dpStatus.data?.enabled) {
		return null;
	}

	if (localDismissed || queryHidden) {
		return null;
	}

	const tier = dpStatus.data.tier ?? 1;
	const isAuditor = dpStatus.data.auditor;
	const infoURL = dpStatus.data.info_url;

	const handleDismiss = () => {
		setLocalDismissed(true);
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
