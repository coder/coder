import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";

// Shared by the sessions list badges and the session detail summary card, which
// render the same two non-numeric states for a session's network requests but
// differ in how they present a live count.

export const NetworkMonitoringDisabled: FC = () => (
	<span className="inline-flex items-center gap-1 whitespace-nowrap text-content-secondary">
		Disabled
		<InfoTooltip message="Network request monitoring was not active for this session." />
	</span>
);

export const NetworkNoActivity: FC = () => (
	<span className="whitespace-nowrap text-content-secondary">No activity</span>
);
