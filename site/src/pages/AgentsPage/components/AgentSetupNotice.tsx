import type { FC } from "react";
import { Link } from "react-router";
import { Alert } from "#/components/Alert/Alert";

const agentSetupLinkClassName =
	"underline transition-colors hover:text-content-primary";

interface AgentSetupNoticeProps {
	noProvidersConfigured: boolean;
	noModelsConfigured: boolean;
}

export const AgentSetupNotice: FC<AgentSetupNoticeProps> = ({
	noProvidersConfigured,
	noModelsConfigured,
}) => {
	if (!noProvidersConfigured && !noModelsConfigured) {
		return null;
	}

	return (
		<Alert severity="warning">
			<div className="flex flex-col gap-2">
				<div>
					<div className="font-medium text-content-primary">
						Finish setting up agents
					</div>
					<div className="mt-1 text-content-secondary">
						Configure a provider and a model before using agents.
					</div>
				</div>
				<div className="flex flex-wrap gap-x-3 gap-y-1">
					{noProvidersConfigured && (
						<Link
							to="/agents/settings/providers"
							className={agentSetupLinkClassName}
						>
							Configure providers
						</Link>
					)}
					{noModelsConfigured && (
						<Link
							to="/agents/settings/models"
							className={agentSetupLinkClassName}
						>
							Configure models
						</Link>
					)}
				</div>
			</div>
		</Alert>
	);
};
