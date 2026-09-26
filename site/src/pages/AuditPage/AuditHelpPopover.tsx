import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { TooltipMessage, TooltipTitle } from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";

export const AuditHelpPopover: FC = () => {
	return (
		<InfoTooltip>
			<TooltipTitle>What is an audit log?</TooltipTitle>
			<TooltipMessage>
				An audit log is a record of events and changes made throughout a system.
				<br />
				<Link size="sm" href={docs("/admin/security/audit-logs")}>
					Events we track
				</Link>
			</TooltipMessage>
		</InfoTooltip>
	);
};
