import type { GetLicensesResponse } from "api/api";
import type { Feature } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import dayjs from "dayjs";
import { ChevronDownIcon, EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import { type FC, useState } from "react";
import { AIGovernanceAddOnCard } from "./AIGovernanceAddOnCard";

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

	const currentUserLimit = license.claims.features.user_limit || userLimitLimit;
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

	const hasExplicitAiGovernanceAddOn = (license.claims.addons ?? []).includes(
		"ai_governance",
	);
	const isAiGovernanceEntitlementInGracePeriod =
		aiGovernanceUserFeature?.entitlement === "grace_period";
	const isLicenseApplicableForAiGovernanceOverage =
		!isNotYetValid && (!isExpired || isAiGovernanceEntitlementInGracePeriod);
	const isActiveAiGovernanceEntitlement =
		aiGovernanceMergedLimit !== undefined &&
		aiGovernanceLimit > 0 &&
		aiGovernanceLimit === aiGovernanceMergedLimit;
	const isAiGovernanceAddOnExceeded =
		isLicenseApplicableForAiGovernanceOverage &&
		hasExplicitAiGovernanceAddOn &&
		isActiveAiGovernanceEntitlement &&
		aiGovernanceActual !== undefined &&
		aiGovernanceActual > aiGovernanceLimit;
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
				? `Starts on ${dayjs.unix(license.claims.nbf ?? 0).format("MMMM D, YYYY")}`
				: "Active";

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
							<div className="flex items-center gap-1.5">
								{isPremium && hasExplicitAiGovernanceAddOn && (
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
										<span className="text-content-primary license-valid-from">
											{dayjs.unix(license.claims.nbf).format("MMMM D, YYYY")}
										</span>
									</div>
								)}
								<div className="flex flex-col items-center">
									<span className="text-content-secondary">Valid Until</span>
									<span className="text-content-primary license-expires">
										{dayjs
											.unix(license.claims.license_expires)
											.format("MMMM D, YYYY")}
									</span>
								</div>
							</div>
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
					{isPremium && hasExplicitAiGovernanceAddOn && (
						<div className="border-0 border-t border-solid border-border bg-surface-primary px-4 py-4">
							<div className="text-sm font-medium text-content-secondary">
								Add-ons
							</div>
							<div className="mt-3 flex flex-wrap gap-3">
								<AIGovernanceAddOnCard
									title="AI governance"
									unit="Seats"
									actual={aiGovernanceActual}
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
