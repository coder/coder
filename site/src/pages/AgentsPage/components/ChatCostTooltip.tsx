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
			size="xsmall"
			side="top"
			ariaLabel="Cost incomplete"
		>
			{/* Widths split each message into two even lines. */}
			<span className="block max-w-68">
				{unpricedRequestCount === 1
					? "1 request"
					: `${unpricedRequestCount} requests`}{" "}
				used a model without configured pricing, so cost may be underreported.
			</span>
		</InfoTooltip>
	) : (
		<InfoTooltip size="xsmall" side="top" ariaLabel="About this cost">
			<span className="block max-w-48">
				Estimated. Updates in increments and may lag slightly.
			</span>
		</InfoTooltip>
	);
