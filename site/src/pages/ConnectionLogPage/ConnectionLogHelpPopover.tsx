import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { TooltipMessage, TooltipTitle } from "#/components/Tooltip/Tooltip";
import { docs } from "#/utils/docs";

export const ConnectionLogHelpPopover: React.FC = () => {
	return (
		<InfoTooltip>
			<TooltipTitle>Why are some events missing?</TooltipTitle>
			<TooltipMessage>
				The connection log is a best-effort log of workspace access. Some events
				are reported by workspace agents, and receipt of these events by the
				server is not guaranteed.
			</TooltipMessage>
			<TooltipTitle className="mt-3">About durations</TooltipTitle>
			<TooltipMessage>
				Durations describe connection intervals, not active work or an IDE's
				lifetime. Historical intervals can be unverified.
			</TooltipMessage>
			<TooltipTitle className="mt-3">Filtering tips</TooltipTitle>
			<TooltipMessage>
				<code>status:ongoing</code> matches connections without a recorded
				disconnect. It does not confirm that a connection is still active.
			</TooltipMessage>
			<TooltipMessage className="mt-1">
				<code>client_session_id:</code> matches an exact 32-character
				hexadecimal client session ID.
			</TooltipMessage>
			<TooltipMessage>
				<Link size="sm" href={docs("/admin/monitoring/connection-logs")}>
					Connection log documentation
				</Link>
			</TooltipMessage>
		</InfoTooltip>
	);
};
