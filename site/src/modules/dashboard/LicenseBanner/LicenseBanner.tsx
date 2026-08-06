import type { FC } from "react";
import {
	LicenseAgentRuntimeHoursClaimsIgnoredWarningText,
	LicenseAgentRuntimeHoursSoftLimitWarningText,
	LicenseAgentRuntimeUsageUnavailableWarningText,
	LicenseAIGovernance90PercentWarningText,
	LicenseAIGovernanceOverLimitWarningText,
	LicenseManagedAgentLimitExceededWarningText,
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

// Diagnostics about the license or the usage measurement rather than about
// usage itself. They point the operator at the coderd logs or at Coder
// support, so they render muted, without the exceedance heading, and without
// a sales link. The "unavailable" pair arrives via entitlements.errors so
// the alertable coderd_license_errors gauge keeps counting measurement
// failures, but nothing about the license is wrong, so they must not render
// as license errors.
const diagnosticMessages: readonly string[] = [
	LicenseManagedAgentUsageUnavailableWarningText,
	LicenseAgentRuntimeUsageUnavailableWarningText,
	LicenseAgentRuntimeHoursClaimsIgnoredWarningText,
];

const isDiagnosticMessage = (message: string): boolean =>
	diagnosticMessages.includes(message);

// Advisory warnings render in the muted variant: nothing is wrong yet, so
// they must be visually distinct from warnings that demand action, such as
// reaching the runtime hours allocation. Diagnostics are muted for the same
// reason, whichever channel they arrive on.
const isMutedWarning = (message: string): boolean =>
	message.startsWith(aiGovernanceNearLimitWarningPrefix) ||
	message.startsWith(agentRuntimeSoftLimitWarningPrefix) ||
	isDiagnosticMessage(message);

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
	// Diagnostics tell the operator to check the logs or contact support; a
	// sales link would contradict them. The advisory soft-limit warning
	// fires inside the purchased allocation with nothing owed, so it gets no
	// sales call-to-action either.
	if (
		isDiagnosticMessage(message) ||
		message.startsWith(agentRuntimeSoftLimitWarningPrefix)
	) {
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
			// Measurement diagnostics travel in the errors channel for the
			// Prometheus gauge but are not license errors; see
			// diagnosticMessages.
			variant: isDiagnosticMessage(message)
				? ("warning" as const)
				: ("error" as const),
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
