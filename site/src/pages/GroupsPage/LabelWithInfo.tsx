import type { InfoTooltipType } from "#/components/InfoTooltip/InfoTooltip";
import { StatusIconTooltip } from "./StatusIconTooltip";

export const LabelWithInfo: React.FC<{
	label: React.ReactNode;
	message: React.ReactNode;
	kind?: InfoTooltipType;
	ariaLabel?: string;
}> = ({ label, message, kind, ariaLabel }) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<StatusIconTooltip message={message} kind={kind} ariaLabel={ariaLabel} />
	</span>
);
