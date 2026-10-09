import { cn } from "cn";
import type React from "react";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";
import { useAgentComposer } from "./AgentComposer";
import { AgentSetupNotice } from "./AgentSetupNotice";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

export const AgentComposerContainer = ({
	children,
	fillWidth = false,
	isEditing = false,
}: {
	children: React.ReactNode;
	fillWidth?: boolean;
	isEditing?: boolean;
}) => {
	const [chatFullWidth] = useChatFullWidth();

	return (
		<div
			className={cn(
				"mx-auto w-full pb-0 sm:pb-4",
				fillWidth ? "max-w-full" : chatWidthClass(chatFullWidth),
				isEditing && "pt-1",
			)}
		>
			{children}
		</div>
	);
};

export type AgentComposerSetup = {
	canConfigureAgentSetup: boolean;
	providerCount?: number;
	modelCount?: number;
	unsupportedProviderNames?: readonly string[];
	aiGatewayDisabled?: boolean;
};

export const needsAgentSetup = (setup: AgentComposerSetup): boolean =>
	Boolean(
		setup.aiGatewayDisabled ||
			(setup.canConfigureAgentSetup
				? setup.providerCount !== undefined &&
					setup.modelCount !== undefined &&
					(setup.providerCount === 0 || setup.modelCount === 0)
				: setup.modelCount !== undefined && setup.modelCount === 0),
	);

export const AgentComposerSetupNotice = ({
	organizationId,
	canConfigureAgentSetup,
	providerCount,
	modelCount,
	unsupportedProviderNames = [],
	aiGatewayDisabled,
}: AgentComposerSetup & { organizationId?: string }) => {
	const { organizations } = useDashboard();
	const organization = organizations.find((org) => org.id === organizationId);

	return (
		<div className="relative z-0 -mb-10">
			<AgentSetupNotice
				isAdmin={canConfigureAgentSetup}
				providerCount={canConfigureAgentSetup ? (providerCount ?? 0) : 0}
				modelCount={canConfigureAgentSetup ? (modelCount ?? 0) : 0}
				organization={organization}
				unsupportedProviderNames={unsupportedProviderNames}
				aiGatewayDisabled={aiGatewayDisabled}
			/>
		</div>
	);
};

export const AgentComposerContextIndicator = (
	props: React.ComponentProps<typeof ContextUsageIndicator>,
) => {
	const { state } = useAgentComposer();

	return (
		<div
			className={cn(
				"flex",
				state.speechSupported && !state.speechError && "-ml-2",
			)}
		>
			<ContextUsageIndicator {...props} />
		</div>
	);
};
