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
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { cn } from "#/utils/cn";
import { AIGovernanceAddOnCard } from "./AIGovernanceAddOnCard";
import {
	isLicenseApplicableForAiGovernanceOverage,
	licenseShowsAiGovernanceAddOn,
} from "./AIGovernanceLicensing";

type LicenseCardProps = {
	license: GetLicensesResponse;
	aiGovernanceUserFeature?: Feature;
	userLimitActual?: number;
	userLimitLimit?: number;
	onRemove: (licenseId: number) => void;
	isRemoving: boolean;
};

export const LicenseCard: FC<LicenseCardProps> = ({
	license,
	aiGovernanceUserFeature,
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

	const licenseType = license.claims.trial
		? "Trial"
		: isPremium
			? "Premium"
			: "Enterprise";

	const hasExplicitAiGovernanceAddOn = licenseShowsAiGovernanceAddOn(license);
	// Overage/display checks only apply to licenses that are currently effective.
	const isLicenseApplicable = isLicenseApplicableForAiGovernanceOverage(
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
	const statusClassName =
		isAiGovernanceAddOnExceeded || isExpired
			? "text-content-destructive"
			: isNotYetValid
				? "text-content-warning"
				: "text-content-success";
	const statusText = isAiGovernanceAddOnExceeded
		? "Add-on exceeded"
		: isExpired
			? "Expired"
			: isNotYetValid
				? "Not started"
				: "Active";
	const hasCollapsibleContent = isPremium && hasExplicitAiGovernanceAddOn;
	const headerContent = (
		<>
			<div className="flex items-center gap-1.5">
				{hasCollapsibleContent && (
					<ChevronDownIcon className="license-chevron size-4 text-content-secondary transition-colors transition-transform group-hover:text-content-primary" />
				)}
				<span className="text-base font-medium text-content-secondary">
					#{license.id}
				</span>
				<span className="account-type text-base font-medium text-content-primary capitalize">
					{licenseType}
				</span>
			</div>

			<div className="ml-auto flex items-center gap-12 text-xs font-medium">
				<div className="flex flex-col items-center">
					<span className="text-content-secondary">Status</span>
					<span className={statusClassName}>{statusText}</span>
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
					{hasCollapsibleContent ? (
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
					) : (
						<div className="m-0 flex min-w-0 flex-1 items-center gap-6">
							{headerContent}
						</div>
					)}

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
					{hasCollapsibleContent && (
						<div className="border-0 border-t border-solid border-border bg-surface-primary px-4 py-4">
							<div className="text-sm font-medium text-content-secondary">
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
						</div>
					)}
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
};
