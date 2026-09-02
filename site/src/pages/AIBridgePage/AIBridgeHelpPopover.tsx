import type { FC } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Link } from "#/components/Link/Link";
import { docs } from "#/utils/docs";

export const AIBridgeHelpPopover: FC = () => {
	return (
		<InfoTooltip
			title="What is AI Gateway?"
			message={
				<>
					AI Gateway is a smart gateway for AI that provides centralized
					management, auditing, and attribution for LLM usage.
					<br />
					<Link size="sm" href={docs("/ai-coder/ai-gateway")}>
						Read the docs
					</Link>
				</>
			}
		/>
	);
};
