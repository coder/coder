import type { FC } from "react";
import { Link } from "react-router";

interface AgentSetupNoticeProps {
	isAdmin: boolean;
	providerCount: number;
	modelCount: number;
}

export const AgentSetupNotice: FC<AgentSetupNoticeProps> = ({
	isAdmin,
	providerCount,
	modelCount,
}) => {
	const hasProvider = providerCount > 0;
	const hasModel = modelCount > 0;

	if (hasProvider && hasModel) {
		return null;
	}

	// Non-admin member: show a generic message
	if (!isAdmin) {
		return (
			<NoticeContainer>
				AI models aren't available yet. Your admin is still getting things set
				up.
			</NoticeContainer>
		);
	}

	// Admin: no providers and no models
	if (!hasProvider && !hasModel) {
		return (
			<NoticeContainer>
				To chat with Coder Agents, set up a{" "}
				<Link
					to="/agents/settings/providers"
					className="text-content-link transition-colors hover:text-content-link/80"
				>
					provider
				</Link>{" "}
				then add a{" "}
				<Link
					to="/agents/settings/models"
					className="text-content-link transition-colors hover:text-content-link/80"
				>
					model
				</Link>
				.
			</NoticeContainer>
		);
	}

	// Admin: has providers but no models
	return (
		<NoticeContainer>
			To chat with Coder Agents, set up a{" "}
			<Link
				to="/agents/settings/models"
				className="text-content-link transition-colors hover:text-content-link/80"
			>
				model
			</Link>
			.
		</NoticeContainer>
	);
};

const NoticeContainer: FC<{ children: React.ReactNode }> = ({ children }) => {
	return (
		<div className="rounded-2xl bg-surface-grey px-4 pb-12 pt-2.5 text-sm text-content-primary">
			{children}
		</div>
	);
};
