import type { GetLicensesResponse } from "api/api";
import { Button } from "components/Button/Button";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Pill } from "components/Pill/Pill";
import dayjs from "dayjs";
import { type FC, useState } from "react";

type LicenseCardProps = {
	license: GetLicensesResponse;
	userLimitActual?: number;
	userLimitLimit?: number;
	onRemove: (licenseId: number) => void;
	isRemoving: boolean;
};

export const LicenseCard: FC<LicenseCardProps> = ({
	license,
	userLimitActual,
	userLimitLimit,
	onRemove,
	isRemoving,
}) => {
	const [licenseIDMarkedForRemoval, setLicenseIDMarkedForRemoval] = useState<
		number | undefined
	>(undefined);

	const currentUserLimit = license.claims.features.user_limit || userLimitLimit;

	const isExpired = dayjs
		.unix(license.claims.license_expires)
		.isBefore(dayjs());

	const licenseType = license.claims.trial
		? "Trial"
		: license.claims.feature_set?.toLowerCase() === "premium"
			? "Premium"
			: "Enterprise";

	return (
		<div
			key={license.id}
			className="license-card rounded-lg border border-solid border-border bg-surface-primary p-4 text-sm shadow-sm"
		>
			<ConfirmDialog
				type="delete"
				hideCancel={false}
				open={licenseIDMarkedForRemoval !== undefined}
				onConfirm={() => {
					if (!licenseIDMarkedForRemoval) {
						return;
					}
					onRemove(licenseIDMarkedForRemoval);
					setLicenseIDMarkedForRemoval(undefined);
				}}
				onClose={() => setLicenseIDMarkedForRemoval(undefined)}
				title="Confirm License Removal"
				confirmLoading={isRemoving}
				confirmText="Remove"
				description={
					isExpired
						? "This license has already expired and is not providing any features. Removing it will not affect your current entitlements."
						: "Removing this license will disable all Premium features. You can add a new license at any time."
				}
			/>
			<div className="flex flex-row gap-4 items-center">
				<span className="text-content-secondary text-lg font-semibold">
					#{license.id}
				</span>
				<span className="account-type font-semibold text-lg capitalize">
					{licenseType}
				</span>
				<div className="flex flex-row justify-end gap-16 items-end flex-1">
					<div className="flex flex-col items-center">
						<span className="text-content-secondary">Users</span>
						<span className="text-content-primary user-limit">
							{userLimitActual} {` / ${currentUserLimit || "Unlimited"}`}
						</span>
					</div>
					{license.claims.nbf && (
						<div className="flex flex-col items-center">
							<span className="text-content-secondary">Valid From</span>
							<span className="text-content-secondary license-valid-from">
								{dayjs.unix(license.claims.nbf).format("MMMM D, YYYY")}
							</span>
						</div>
					)}
					<div className="flex flex-col items-center">
						{isExpired ? (
							<Pill className="mb-1" type="error">
								Expired
							</Pill>
						) : (
							<span className="text-content-secondary">Valid Until</span>
						)}
						<span className="text-content-secondary license-expires">
							{dayjs
								.unix(license.claims.license_expires)
								.format("MMMM D, YYYY")}
						</span>
					</div>
					<Button
						variant="destructive"
						size="sm"
						onClick={() => setLicenseIDMarkedForRemoval(license.id)}
						className="remove-button"
					>
						Remove&hellip;
					</Button>
				</div>
			</div>
		</div>
	);
};
