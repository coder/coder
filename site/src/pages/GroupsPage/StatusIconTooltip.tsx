import {
	InfoTooltip,
	type InfoTooltipType,
} from "#/components/InfoTooltip/InfoTooltip";
import { TooltipMessage } from "#/components/Tooltip/Tooltip";

/** A hover tooltip anchored to a status icon, styled per `kind`. */
export const StatusIconTooltip: React.FC<{
	message: React.ReactNode;
	kind?: InfoTooltipType;
}> = ({ message, kind = "info" }) => (
	<InfoTooltip type={kind} size="small">
		<TooltipMessage>{message}</TooltipMessage>
	</InfoTooltip>
);
