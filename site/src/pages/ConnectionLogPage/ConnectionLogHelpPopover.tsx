import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const ConnectionLogHelpPopover: FC = () => {
	return (
		<InfoTooltip
			title="Why are some events missing?"
			message={
				<>
					The connection log is a best-effort log of workspace access. Some
					events are reported by workspace agents, and receipt of these events
					by the server is not guaranteed.
					<br />
					<Link size="sm" href={docs("/admin/monitoring/connection-logs")}>
						Connection log documentation
					</Link>
				</>
			}
		/>
	);
};
