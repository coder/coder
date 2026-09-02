import type { FC, ReactNode } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";

/** A hover tooltip anchored to a status icon, styled per `kind`. */
export const StatusIconTooltip: FC<{
	message: ReactNode;
	kind?: "info" | "warning";
}> = ({ message, kind = "info" }) => (
	<InfoTooltip type={kind} size="small" message={message} />
);
