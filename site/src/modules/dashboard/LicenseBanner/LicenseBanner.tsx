import type { FC } from "react";
import {
	LicenseAgentRuntimeHoursSoftLimitWarningText,
	LicenseAgentRuntimeUsageNotConfiguredWarningText,
	LicenseAgentRuntimeUsageUnavailableWarningText,
	LicenseAIGovernance90PercentWarningText,
	LicenseAIGovernanceOverLimitWarningText,
	LicenseManagedAgentLimitExceededWarningText,
	LicenseManagedAgentUsageNotConfiguredWarningText,
	LicenseManagedAgentUsageUnavailableWarningText,
	LicenseTelemetryRequiredErrorText,
} from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { docs } from "#/utils/docs";
import {
	type LicenseBannerLink,
	type LicenseBannerMessage,
	LicenseBannerView,
} from "./LicenseBannerView";

const aiGovernanceOverLimitWarningPrefix =
	LicenseAIGovernanceOverLimitWarningText.split("%d")[0];
const aiGovernanceNearLimitWarningPrefix =
	LicenseAIGovernance90PercentWarningText.split("%d%%")[0];
const agentRuntimeSoftLimitWarningPrefix =
	LicenseAgentRuntimeHoursSoftLimitWarningText.split("%d")[0];
const AI_GOVERNANCE_NEAR_LIMIT_FALLBACK_MESSAGE =
	"You are approaching your AI Governance add-on seat limit.";

const isAIGovernanceWarning = (message: string): boolean =>
	message.startsWith(aiGovernanceNearLimitWarningPrefix) ||
	message.startsWith(aiGovernanceOverLimitWarningPrefix);

// Advisory warnings render in the muted variant: nothing is wrong yet, so
// they must be visually distinct from warnings that demand action, such as
// reaching the runtime hours allocation.
const isMutedWarning = (message: string): boolean =>
	message.startsWith(aiGovernanceNearLimitWarningPrefix) ||
	message.startsWith(agentRuntimeSoftLimitWarningPrefix);

const aiGovernanceOverLimitMessage = (
	feature: ReturnType<
		typeof useDashboard
	>["entitlements"]["features"]["ai_governance_user_limit"],
): string | null => {
	if (!feature) {
		return null;
	}

	const { actual, entitlement, limit } = feature;
	if (
		(entitlement !== "entitled" && entitlement !== "grace_period") ||
		actual === undefined ||
		limit === undefined ||
		limit <= 0 ||
		actual <= limit
	) {
		return null;
	}

	const overLimitSeats = actual - limit;
	return LicenseAIGovernanceOverLimitWarningText.replace("%d", `${actual}`)
		.replace("%d", `${limit}`)
		.replace("%d", `${overLimitSeats}`);
};

const aiGovernanceNearLimitMessage = (
	feature: ReturnType<
		typeof useDashboard
	>["entitlements"]["features"]["ai_governance_user_limit"],
): string | null => {
	if (!feature) {
		return null;
	}

	const { actual, entitlement, limit } = feature;
	if (
		(entitlement !== "entitled" && entitlement !== "grace_period") ||
		actual === undefined ||
		limit === undefined ||
		limit <= 0
	) {
		return null;
	}

	const usedPercent = Math.floor((actual * 100) / limit);
	if (usedPercent < 90) {
		return null;
	}

	return LicenseAIGovernance90PercentWarningText.replace(
		"%d%%",
		`${usedPercent}%`,
	);
};

const normalizeAIGovernanceWarning = (
	message: string,
	feature: ReturnType<
		typeof useDashboard
	>["entitlements"]["features"]["ai_governance_user_limit"],
): string => {
	if (message !== LicenseAIGovernance90PercentWarningText) {
		return message;
	}

	return (
		aiGovernanceNearLimitMessage(feature) ??
		AI_GOVERNANCE_NEAR_LIMIT_FALLBACK_MESSAGE
	);
};

// Usage-measurement diagnostics point the operator at the coderd logs or at
// a Coder bug; a sales link under them would contradict the message.
const diagnosticWarnings: readonly string[] = [
	LicenseManagedAgentUsageUnavailableWarningText,
	LicenseManagedAgentUsageNotConfiguredWarningText,
	LicenseAgentRuntimeUsageUnavailableWarningText,
	LicenseAgentRuntimeUsageNotConfiguredWarningText,
];

const messageLink = (message: string): LicenseBannerLink | undefined => {
	if (message === LicenseManagedAgentLimitExceededWarningText) {
		return {
			href: docs("/ai-coder/ai-governance"),
			label: "View AI Governance",
			showExternalIcon: true,
			target: "_blank",
		};
	}
	if (message === LicenseTelemetryRequiredErrorText) {
		return {
			href: "mailto:sales@coder.com",
			label: "Contact sales@coder.com if you need an exception.",
			showExternalIcon: false,
		};
	}
	if (diagnosticWarnings.includes(message)) {
		return undefined;
	}
	return {
		href: "mailto:sales@coder.com",
		label: "Contact sales@coder.com.",
		showExternalIcon: false,
	};
};

export const LicenseBanner: FC = () => {
	const { entitlements } = useDashboard();
	const { errors } = entitlements;
	const warnings = [...entitlements.warnings];
	const aiGovernanceUserLimitFeature =
		entitlements.features.ai_governance_user_limit;
	const overLimitWarning = aiGovernanceOverLimitMessage(
		aiGovernanceUserLimitFeature,
	);

	if (
		overLimitWarning &&
		!warnings.some((warning) => isAIGovernanceWarning(warning))
	) {
		warnings.push(overLimitWarning);
	}

	const normalizedWarnings = warnings.map((warning) =>
		normalizeAIGovernanceWarning(warning, aiGovernanceUserLimitFeature),
	);

	const messages: LicenseBannerMessage[] = [
		...errors.map((message) => ({
			message,
			variant: "error" as const,
			link: messageLink(message),
		})),
		...normalizedWarnings.map((message) => ({
			message,
			variant: isMutedWarning(message)
				? ("warning" as const)
				: ("warningProminent" as const),
			link: messageLink(message),
		})),
	];

	if (messages.length === 0) {
		return null;
	}

	return <LicenseBannerView messages={messages} />;
};
