import type { FC, ReactNode } from "react";
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

	// Admin: missing provider (with or without models)
	if (!hasProvider) {
		return (
			<NoticeContainer>
				To chat with Coder Agents, set up a{" "}
				<Link
					to="/ai/settings"
					className="text-content-link transition-colors hover:text-content-link/80"
				>
					provider
				</Link>
				{!hasModel && (
					<>
						{" "}
						then add a{" "}
						<Link
							to="/ai/settings/models"
							className="text-content-link transition-colors hover:text-content-link/80"
						>
							model
						</Link>
					</>
				)}
				.
			</NoticeContainer>
		);
	}

	// Admin: has providers but no models
	return (
		<NoticeContainer>
			To chat with Coder Agents, set up a{" "}
			<Link
				to="/ai/settings/models"
				className="text-content-link transition-colors hover:text-content-link/80"
			>
				model
			</Link>
			.
		</NoticeContainer>
	);
};

const NoticeContainer: FC<{ children: ReactNode }> = ({ children }) => {
	return (
		<div className="rounded-2xl bg-surface-tertiary px-4 pb-14 pt-2.5 text-[13px] text-content-primary">
			{children}
		</div>
	);
};
