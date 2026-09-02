import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const AuditHelpPopover: FC = () => {
	return (
		<InfoTooltip
			title="What is an audit log?"
			message={
				<>
					An audit log is a record of events and changes made throughout a
					system.
					<br />
					<Link size="sm" href={docs("/admin/security/audit-logs")}>
						Events we track
					</Link>
				</>
			}
		/>
	);
};
