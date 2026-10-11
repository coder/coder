import type { InfoTooltipType } from "#/components/InfoTooltip/InfoTooltip";
import { StatusIconTooltip } from "./StatusIconTooltip";

type LabelWithInfoProps = {
	label: React.ReactNode;
	message: React.ReactNode;
	kind?: InfoTooltipType;
	ariaLabel?: string;
};

export const LabelWithInfo: React.FC<LabelWithInfoProps> = ({
	label,
	message,
	kind,
	ariaLabel,
}) => (
	<span className="inline-flex items-center gap-1">
		{label}
		<StatusIconTooltip message={message} kind={kind} ariaLabel={ariaLabel} />
	</span>
);
