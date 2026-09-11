import type { FC, ReactNode } from "react";
import {
	InfoTooltip,
	type InfoTooltipType,
} from "#/components/InfoTooltip/InfoTooltip";
import { TooltipMessage } from "#/components/Tooltip/Tooltip";

/** A hover tooltip anchored to a status icon, styled per `kind`. */
export const StatusIconTooltip: FC<{
	message: ReactNode;
	kind?: InfoTooltipType;
}> = ({ message, kind = "info" }) => (
	<InfoTooltip type={kind} size="small">
		<TooltipMessage>{message}</TooltipMessage>
	</InfoTooltip>
);
