import type { FC } from "react";
import {
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

const isAIGovernanceWarning = (message: string): boolean =>
	message === LicenseAIGovernance90PercentWarningText ||
	message.startsWith(aiGovernanceOverLimitWarningPrefix);

const isAIGovernanceNearLimitWarning = (message: string): boolean =>
	message === LicenseAIGovernance90PercentWarningText;

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

	const overPercent = Math.max(1, Math.floor(((actual - limit) / limit) * 100));
	return `Your organization is using ${actual} / ${limit} AI Governance user seats (${overPercent}% over the limit)`;
};

const messageLink = (message: string): LicenseBannerLink => {
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
	const overLimitWarning = aiGovernanceOverLimitMessage(
		entitlements.features.ai_governance_user_limit,
	);

	if (
		overLimitWarning &&
		!warnings.some((warning) => isAIGovernanceWarning(warning))
	) {
		warnings.push(overLimitWarning);
	}

	const messages: LicenseBannerMessage[] = [
		...errors.map((message) => ({
			message,
			variant: "error" as const,
			link: messageLink(message),
		})),
		...warnings.map((message) => ({
			message,
			variant: isAIGovernanceNearLimitWarning(message)
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
