import type { FC } from "react";
import {
	LicenseAgentRuntimeHoursClaimsIgnoredWarningText,
	LicenseAgentRuntimeHoursSoftLimitWarningText,
	LicenseAgentRuntimeUsageUnavailableErrorText,
	LicenseAIGovernance90PercentWarningText,
	LicenseAIGovernanceOverLimitWarningText,
	LicenseManagedAgentLimitExceededWarningText,
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

// Substitutes the given values into the template's %d placeholders in order.
// No other fmt verb, width, or flag is implemented. Exported for the
// stories, so what they pin is what production renders.
export const formatLicenseMessage = (
	template: string,
	...values: number[]
): string =>
	values.reduce(
		(message, value) => message.replace("%d", `${value}`),
		template,
	);

// Diagnostics about the license or the usage measurement rather than about
// usage itself. They render muted, without the exceedance heading or a sales
// link, even when they arrive via entitlements.errors.
const diagnosticMessages: readonly string[] = [
	LicenseAgentRuntimeUsageUnavailableErrorText,
	LicenseAgentRuntimeHoursClaimsIgnoredWarningText,
];

const isDiagnosticMessage = (message: string): boolean =>
	diagnosticMessages.includes(message);

// Advisories and diagnostics render muted to stay visually distinct from
// warnings that demand action, such as reaching the runtime hours
// allocation.
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
	return formatLicenseMessage(
		LicenseAIGovernanceOverLimitWarningText,
		actual,
		limit,
		overLimitSeats,
	);
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
	// Diagnostics point the operator at the logs or support, and the
	// soft-limit advisory fires inside the purchased allocation, so neither
	// gets a sales link.
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
		...errors.map(
			(message): LicenseBannerMessage => ({
				message,
				// Measurement diagnostics travel in the errors channel but are
				// not license errors; see diagnosticMessages.
				variant: isDiagnosticMessage(message) ? "warning" : "error",
				link: messageLink(message),
			}),
		),
		...normalizedWarnings.map(
			(message): LicenseBannerMessage => ({
				message,
				variant: isMutedWarning(message) ? "warning" : "warningProminent",
				link: messageLink(message),
			}),
		),
	];

	if (messages.length === 0) {
		return null;
	}

	return <LicenseBannerView messages={messages} />;
};
