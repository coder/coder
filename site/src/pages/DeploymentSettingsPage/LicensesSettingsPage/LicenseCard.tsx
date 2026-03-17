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
import { Pill } from "components/Pill/Pill";
import dayjs from "dayjs";
import { ChevronDownIcon, EllipsisVerticalIcon, TrashIcon } from "lucide-react";
import { type FC, useState } from "react";
import { AIGovernanceAddOnCard } from "./AIGovernanceAddOnCard";

type LicenseCardProps = {
	license: GetLicensesResponse;
	includedWithPremium: number;
	additionalPurchased: number;
	aiGovernanceUserFeature?: Feature;
	userLimitActual?: number;
	userLimitLimit?: number;
	onRemove: (licenseId: number) => void;
	isRemoving: boolean;
};

export const LicenseCard: FC<LicenseCardProps> = ({
	license,
	includedWithPremium,
	additionalPurchased,
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
	const isPremium = license.claims.feature_set?.toLowerCase() === "premium";
	const aiGovernanceActual = aiGovernanceUserFeature?.actual ?? 0;
	const aiGovernanceLimit = aiGovernanceUserFeature?.limit ?? 0;

	const licenseType = license.claims.trial
		? "Trial"
		: isPremium
			? "Premium"
			: "Enterprise";

	const hasAiGovernanceAddOn =
		(aiGovernanceUserFeature?.enabled ?? false) &&
		((license.claims.addons ?? []).includes("ai_governance") ||
			(license.claims.features?.ai_governance_user_limit ?? 0) > 0);

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
								<ChevronDownIcon className="license-chevron size-4 text-content-secondary transition-colors transition-transform group-hover:text-content-primary" />
								<span className="text-base font-medium text-content-secondary">
									#{license.id}
								</span>
								<span className="account-type text-base font-medium capitalize">
									{licenseType}
								</span>
							</div>

							<div className="ml-auto flex items-center gap-12 text-xs font-medium">
								<div className="flex flex-col items-start">
									<span className="text-content-secondary">Status</span>
									<span
										className={
											isExpired
												? "text-content-destructive"
												: "text-content-success"
										}
									>
										{isExpired ? "Expired" : "Active"}
									</span>
								</div>
								<div className="flex flex-col items-start">
									<span className="text-content-secondary">Users</span>
									<span className="text-content-primary user-limit">
										{userLimitActual} {` / ${currentUserLimit || "Unlimited"}`}
									</span>
								</div>
								{license.claims.nbf && (
									<div className="flex flex-col items-start">
										<span className="text-content-secondary">Valid From</span>
										<span className="text-content-primary license-valid-from">
											{dayjs.unix(license.claims.nbf).format("MMMM D, YYYY")}
										</span>
									</div>
								)}
								<div className="flex flex-col items-start">
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
					{isPremium && hasAiGovernanceAddOn && (
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
									includedWithPremium={includedWithPremium}
									additionalPurchased={additionalPurchased}
								/>
							</div>
						</div>
					)}
					{isExpired && (
						<div className="border-0 border-t border-solid border-border px-4 py-3">
							<Pill type="error">Expired</Pill>
						</div>
					)}
				</CollapsibleContent>
			</div>
		</Collapsible>
	);
};
