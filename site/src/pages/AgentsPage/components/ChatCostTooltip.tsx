import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";

type ChatCostTooltipProps = {
	unpricedRequestCount: number;
};

/** Explains that a chat cost is an estimate, or a lower bound when usage is unpriced. */
export const ChatCostTooltip: React.FC<ChatCostTooltipProps> = ({
	unpricedRequestCount,
}) =>
	unpricedRequestCount > 0 ? (
		<InfoTooltip
			type="warning"
			size="small"
			side="top"
			ariaLabel="Cost incomplete"
		>
			Pricing isn't fully configured for this model, so cost may be
			underreported.
		</InfoTooltip>
	) : (
		<InfoTooltip size="small" side="top" ariaLabel="About this cost">
			Estimated. Updates in increments and may lag slightly.
		</InfoTooltip>
	);
