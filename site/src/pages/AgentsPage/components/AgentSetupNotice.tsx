import type { FC, ReactNode } from "react";
import { Link } from "react-router";
import { docs } from "#/utils/docs";

interface AgentSetupNoticeProps {
	isAdmin: boolean;
	providerCount: number;
	modelCount: number;
	// Names of configured providers the harness cannot use, populated by
	// the page only when no supported provider is configured.
	unsupportedProviderNames?: readonly string[];
	aiGatewayDisabled?: boolean;
}

const formatProviderList = (names: readonly string[]): string => {
	if (names.length === 1) {
		return names[0];
	}
	if (names.length === 2) {
		return `${names[0]} and ${names[1]}`;
	}
	return `${names.slice(0, -1).join(", ")}, and ${names[names.length - 1]}`;
};

export const AgentSetupNotice: FC<AgentSetupNoticeProps> = ({
	isAdmin,
	providerCount,
	modelCount,
	unsupportedProviderNames = [],
	aiGatewayDisabled,
}) => {
	const hasProvider = providerCount > 0;
	const hasModel = modelCount > 0;
	const hasUnsupportedProviderNames = unsupportedProviderNames.length > 0;

	// AI Gateway can be disabled even when providers/models exist in the DB
	// catalog, so check it before the provider/model counts below. Unlike
	// the provider/model branches, there is no in-app settings page for
	// this deployment-level flag for any audience, so the message doesn't
	// vary by isAdmin.
	if (aiGatewayDisabled) {
		return (
			<NoticeContainer>
				AI Gateway is disabled. Enable it in your deployment config to chat with
				Coder Agents.
			</NoticeContainer>
		);
	}

	if (hasProvider && hasModel) {
		return null;
	}

	// Configured providers exist but none are supported by Coder Agents
	// (e.g. GitHub Copilot). Say so rather than asking to set up a provider.
	if (hasUnsupportedProviderNames) {
		const providerList = formatProviderList(unsupportedProviderNames);
		const unsupportedLink = (
			<a
				href={docs("/ai-coder/agents/models#providers")}
				target="_blank"
				rel="noreferrer"
				className="text-content-link transition-colors hover:text-content-link/80"
			>
				not supported by Coder Agents
			</a>
		);
		if (!isAdmin) {
			return (
				<NoticeContainer>
					{providerList} {unsupportedProviderNames.length === 1 ? "is" : "are"}{" "}
					configured but {unsupportedLink}. Ask your admin to add a supported
					provider.
				</NoticeContainer>
			);
		}
		return (
			<NoticeContainer>
				{providerList} {unsupportedProviderNames.length === 1 ? "is" : "are"}{" "}
				configured but {unsupportedLink}. Add a supported{" "}
				<Link
					to="/ai/settings/providers"
					className="text-content-link transition-colors hover:text-content-link/80"
				>
					provider
				</Link>{" "}
				to chat with Coder Agents.
			</NoticeContainer>
		);
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
					to="/ai/settings/providers"
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
