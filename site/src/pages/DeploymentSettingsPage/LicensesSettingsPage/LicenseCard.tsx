import dayjs from "dayjs";
import { ChevronDownIcon, EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { GetLicensesResponse } from "#/api/api";
import type { Feature } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "#/components/Collapsible/Collapsible";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { cn } from "#/utils/cn";
import { AIGovernanceAddOnCard } from "./AIGovernanceAddOnCard";
import { licenseShowsAiGovernanceAddOn } from "./AIGovernanceLicensing";
import { CoderAgentsProductCard } from "./CoderAgentsProductCard";
import { CoderWorkspacesProductCard } from "./CoderWorkspacesProductCard";
import { isLicenseApplicableForFeatureUsage } from "./licenseApplicability";

type LicenseCardProps = {
	license: GetLicensesResponse;
	aiGovernanceUserFeature?: Feature;
	agentRuntimeHoursFeature?: Feature;
	userLimitActual?: number;
	userLimitLimit?: number;
	onRemove: (licenseId: number) => void;
	isRemoving: boolean;
};

export const LicenseCard: FC<LicenseCardProps> = ({
	license,
	aiGovernanceUserFeature,
	agentRuntimeHoursFeature,
	userLimitActual,
	userLimitLimit,
	onRemove,
	isRemoving,
}) => {
	const [licenseIDMarkedForRemoval, setLicenseIDMarkedForRemoval] = useState<
		number | undefined
	>(undefined);

	const currentUserLimit = license.claims.features.user_limit ?? userLimitLimit;
	const confirmationName = licenseIDMarkedForRemoval?.toString() ?? "";

	const isExpired = dayjs
		.unix(license.claims.license_expires)
		.isBefore(dayjs());
	const isNotYetValid =
		license.claims.nbf !== undefined &&
		dayjs.unix(license.claims.nbf).isAfter(dayjs());
	const isPremium = license.claims.feature_set?.toLowerCase() === "premium";
	const aiGovernanceActual = aiGovernanceUserFeature?.actual;
	const aiGovernanceMergedLimit = aiGovernanceUserFeature?.limit;
	const aiGovernanceLimit =
		license.claims.features?.ai_governance_user_limit ?? 0;

	const licenseType = isPremium ? "Premium" : "Enterprise";

	const hasExplicitAiGovernanceAddOn = licenseShowsAiGovernanceAddOn(license);
	// Overage/display checks only apply to licenses that are currently effective.
	const isLicenseApplicable = isLicenseApplicableForFeatureUsage(
		license,
		aiGovernanceUserFeature,
	);
	// A license "wins" when its AI Governance limit matches the merged limit.
	const isWinningAiGovernanceLicense =
		aiGovernanceMergedLimit !== undefined &&
		aiGovernanceLimit > 0 &&
		aiGovernanceLimit === aiGovernanceMergedLimit;
	const canUseAiGovernanceUsageForThisLicense =
		isLicenseApplicable &&
		hasExplicitAiGovernanceAddOn &&
		isWinningAiGovernanceLicense;
	// Show the add-on as exceeded only for the winning, active add-on license.
	const isAiGovernanceAddOnExceeded =
		canUseAiGovernanceUsageForThisLicense &&
		aiGovernanceActual !== undefined &&
		aiGovernanceActual > aiGovernanceLimit;
	// Show actual usage only when this license is the one providing the limit.
	const aiGovernanceDisplayActual = canUseAiGovernanceUsageForThisLicense
		? aiGovernanceActual
		: undefined;

	// Agent runtime hour claims, in hours. The -1 allocation is the
	// unlimited sentinel; other negative allocations are ignored by the
	// backend, and a zero allocation grants the feature disabled.
	const agentHoursAllocation =
		license.claims.features.agent_runtime_hours_allocation;
	// The backend decodes runtime hour claims for every license
	// regardless of feature set, so an Enterprise license carrying a
	// usable allocation claim also gets the Coder Agents product.
	// Premium licenses without claims are grandfathered into the
	// zero-hour (upgrade) display.
	const hasAgentHoursClaim =
		agentHoursAllocation !== undefined &&
		(agentHoursAllocation >= 0 || agentHoursAllocation === -1);
	const licenseGrantsAgentHours =
		agentHoursAllocation !== undefined &&
		(agentHoursAllocation > 0 || agentHoursAllocation === -1);
	// Thresholds after the backend's claim validation: soft must be
	// non-negative and below a positive allocation, hard at or above it.
	// Invalid threshold claims are ignored rather than disqualifying the
	// license.
	const agentHoursSoftLimitClaim =
		license.claims.features.agent_runtime_hours_limit_soft;
	const agentHoursHardLimitClaim =
		license.claims.features.agent_runtime_hours_limit_hard;
	const agentHoursSoftLimit =
		agentHoursAllocation !== undefined &&
		agentHoursAllocation > 0 &&
		agentHoursSoftLimitClaim !== undefined &&
		agentHoursSoftLimitClaim >= 0 &&
		agentHoursSoftLimitClaim < agentHoursAllocation
			? agentHoursSoftLimitClaim
			: undefined;
	const agentHoursHardLimit =
		agentHoursAllocation !== undefined &&
		agentHoursAllocation > 0 &&
		agentHoursHardLimitClaim !== undefined &&
		agentHoursHardLimitClaim >= agentHoursAllocation
			? agentHoursHardLimitClaim
			: undefined;
	const isAgentHoursLicenseApplicable = isLicenseApplicableForFeatureUsage(
		license,
		agentRuntimeHoursFeature,
	);
	// The merged entitlement's usage period is copied verbatim from the
	// license the backend selected (issued_at from iat, start from nbf,
	// end from exp), so a license only "wins" when all three match.
	// Feature.Compare tie-breaks equal issued-at values on the period
	// end, so matching issued-at alone could mark two licenses with the
	// same second-granularity iat as the winner.
	const mergedUsagePeriod = agentRuntimeHoursFeature?.usage_period;
	const matchesMergedUsagePeriod =
		license.claims.iat !== undefined &&
		license.claims.nbf !== undefined &&
		license.claims.exp !== undefined &&
		mergedUsagePeriod !== undefined &&
		dayjs.unix(license.claims.iat).isSame(mergedUsagePeriod.issued_at) &&
		dayjs.unix(license.claims.nbf).isSame(mergedUsagePeriod.start) &&
		dayjs.unix(license.claims.exp).isSame(mergedUsagePeriod.end);
	// Beyond the usage period, the license's allocation and validated
	// thresholds must equal the merged entitlement's: equal limits (or an
	// unlimited allocation with the merged limit omitted) and equal
	// soft/hard thresholds, since the backend retains only the selected
	// license's thresholds.
	const isWinningAgentHoursLicense =
		matchesMergedUsagePeriod &&
		agentHoursSoftLimit === agentRuntimeHoursFeature?.soft_limit &&
		agentHoursHardLimit === agentRuntimeHoursFeature?.hard_limit &&
		(agentHoursAllocation === -1
			? agentRuntimeHoursFeature?.enabled === true &&
				agentRuntimeHoursFeature.limit === undefined
			: agentHoursAllocation !== undefined &&
				agentHoursAllocation > 0 &&
				agentHoursAllocation === agentRuntimeHoursFeature?.limit);
	const canUseAgentHoursUsageForThisLicense =
		isAgentHoursLicenseApplicable && isWinningAgentHoursLicense;
	// Precise usage in tenths of hours, floored via integer math so the
	// displayed number and the exceeded state below flip at the same
	// instant as the backend's whole-hour warning thresholds.
	const agentHoursActualMs = agentRuntimeHoursFeature?.actual_ms;
	const agentHoursActual =
		agentHoursActualMs === undefined
			? undefined
			: Math.floor(agentHoursActualMs / 360_000) / 10;
	// Usage applies to the winning license's quota. Licenses without an
	// allocation show deployment-wide usage in their upgrade card instead.
	const agentHoursDisplayActual =
		isAgentHoursLicenseApplicable &&
		(isWinningAgentHoursLicense || !licenseGrantsAgentHours)
			? agentHoursActual
			: undefined;
	const isAgentHoursHardLimitExceeded =
		canUseAgentHoursUsageForThisLicense &&
		agentHoursHardLimit !== undefined &&
		agentHoursDisplayActual !== undefined &&
		agentHoursDisplayActual >= agentHoursHardLimit;
	const isAgentHoursExceeded =
		canUseAgentHoursUsageForThisLicense &&
		!isAgentHoursHardLimitExceeded &&
		agentHoursAllocation !== undefined &&
		agentHoursAllocation > 0 &&
		agentHoursDisplayActual !== undefined &&
		agentHoursDisplayActual > agentHoursAllocation;

	const statusClassName =
		isAgentHoursHardLimitExceeded ||
		isAgentHoursExceeded ||
		isAiGovernanceAddOnExceeded ||
		isExpired
			? "text-content-destructive"
			: isNotYetValid
				? "text-content-warning"
				: "text-content-success";
	const statusText = isAgentHoursHardLimitExceeded
		? "Hard limit exceeded"
		: isAgentHoursExceeded
			? "Agent hours exceeded"
			: isAiGovernanceAddOnExceeded
				? "Add-on exceeded"
				: isExpired
					? "Expired"
					: isNotYetValid
						? "Not started"
						: "Active";
	const includesAgents =
		Boolean(license.claims.trial) || licenseGrantsAgentHours;
	const includedProducts = isPremium
		? [
				"Workspaces",
				...(hasExplicitAiGovernanceAddOn ? ["AI Governance"] : []),
				...(includesAgents ? ["Agents"] : []),
			]
		: [];
	const includedProductsLabel = includedProducts.join(" + ");
	const headerContent = (
		<>
			<div className="flex items-start gap-1.5">
				<ChevronDownIcon className="license-chevron mt-1 size-4 shrink-0 text-content-secondary transition-colors transition-transform group-hover:text-content-primary" />
				<span className="text-base font-medium text-content-secondary">
					#{license.id}
				</span>
				<div className="flex min-w-0 flex-col">
					<span className="account-type text-base font-medium text-content-primary capitalize">
						{licenseType}
					</span>
					{includedProducts.length > 0 && (
						<div
							role="group"
							aria-label={includedProductsLabel}
							className="text-xs font-medium text-content-secondary"
						>
							{includedProducts.map((product, index) => (
								<span key={product}>
									{index > 0 && (
										<span className="text-highlight-purple"> + </span>
									)}
									{product}
								</span>
							))}
						</div>
					)}
				</div>
			</div>

			<div className="ml-auto flex items-center gap-12 text-xs font-medium">
				<div className="flex flex-col items-center">
					<span className="text-content-secondary">Status</span>
					<span className={statusClassName}>{statusText}</span>
				</div>
				<div className="flex flex-col items-center">
					<span className="text-content-secondary">Type</span>
					<span className="license-type text-content-primary">
						{license.claims.trial ? "Trial" : "Standard"}
					</span>
				</div>
				<div className="flex flex-col items-center">
					<span className="text-content-secondary">Users</span>
					<span className="text-content-primary user-limit">
						{userLimitActual} {` / ${currentUserLimit || "Unlimited"}`}
					</span>
				</div>
				{license.claims.nbf && (
					<div className="flex flex-col items-center">
						<span className="text-content-secondary">Valid From</span>
						<span
							className={cn("license-valid-from", {
								"text-content-warning": statusText === "Not started",
								"text-content-primary": statusText !== "Not started",
							})}
						>
							{dayjs.unix(license.claims.nbf).format("MMMM D, YYYY")}
						</span>
					</div>
				)}
				<div className="flex flex-col items-center">
					<span className="text-content-secondary">Valid Until</span>
					<span className="text-content-primary license-expires">
						{dayjs.unix(license.claims.license_expires).format("MMMM D, YYYY")}
					</span>
				</div>
			</div>
		</>
	);

	return (
		<Collapsible defaultOpen>
			<DeleteDialog
				key={licenseIDMarkedForRemoval}
				isOpen={licenseIDMarkedForRemoval !== undefined}
				onConfirm={() => {
					if (!licenseIDMarkedForRemoval) return;
					onRemove(licenseIDMarkedForRemoval);
					setLicenseIDMarkedForRemoval(undefined);
				}}
				onCancel={() => setLicenseIDMarkedForRemoval(undefined)}
				entity="license"
				name={confirmationName}
				label="ID of the license to remove"
				title="Confirm license removal"
				verb="Removing"
				confirmText="Remove"
				info={
					isExpired
						? "This license has already expired and is not providing any features. Removing it will not affect your current entitlements."
						: "Removing this license will disable all Premium features. You can add a new license at any time."
				}
				confirmLoading={isRemoving}
			/>
			<div className="license-card group overflow-hidden rounded-md border border-solid border-border bg-surface-secondary text-sm shadow-sm">
				<div className="flex items-center gap-6 p-3">
					<CollapsibleTrigger
						asChild
						className="[&[data-state=closed]_.license-chevron]:-rotate-90"
					>
						<button
							type="button"
							className="m-0 flex min-w-0 flex-1 appearance-none items-center gap-6 border-0 bg-transparent p-0 text-left"
						>
							{headerContent}
						</button>
					</CollapsibleTrigger>

					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								onClick={(event) => event.stopPropagation()}
								className="size-[30px]"
							>
								<EllipsisVerticalIcon />
								<span className="sr-only">Show license actions</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onClick={() => setLicenseIDMarkedForRemoval(license.id)}
							>
								<TrashIcon />
								Remove&hellip;
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>

				<CollapsibleContent>
					<div className="border-0 border-t border-solid border-border bg-surface-primary px-4 py-4">
						<div className="text-sm font-medium text-content-secondary">
							Products
						</div>
						<div className="mt-3 flex flex-wrap gap-3">
							<CoderWorkspacesProductCard
								userLimitActual={userLimitActual}
								userLimitLimit={currentUserLimit}
							/>
							{(isPremium || hasAgentHoursClaim) && (
								<CoderAgentsProductCard
									allocation={agentHoursAllocation}
									actual={agentHoursDisplayActual}
									isExceeded={isAgentHoursExceeded}
									isHardLimitExceeded={isAgentHoursHardLimitExceeded}
								/>
							)}
						</div>
						{hasExplicitAiGovernanceAddOn && (
							<>
								<div className="mt-4 text-sm font-medium text-content-secondary">
									Add-ons
								</div>
								<div className="mt-3 flex flex-wrap gap-3">
									<AIGovernanceAddOnCard
										title="AI Governance"
										unit="Seats"
										actual={aiGovernanceDisplayActual}
										limit={aiGovernanceLimit}
										isExceeded={isAiGovernanceAddOnExceeded}
									/>
								</div>
							</>
						)}
					</div>
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
};
