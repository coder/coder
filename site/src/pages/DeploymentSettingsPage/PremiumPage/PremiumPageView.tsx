import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import {
	PaywallGuidance,
	PaywallTitle,
	PREMIUM_DEFAULT_HERO,
} from "#/components/Paywall/Paywall";
import { Supergraphic } from "#/components/Supergraphic/Supergraphic";
import { LicenseActivePanel } from "./LicenseActivePanel";
import { TrialActivePanel } from "./TrialActivePanel";
import { TrialRequestForm } from "./TrialRequestForm";

interface PremiumPageViewProps {
	hasLicense: boolean;
	isTrial: boolean;
	/** Whether the viewer may request a trial for this deployment. */
	canRequestTrial: boolean;
	trialDaysRemaining: number | undefined;
	isLoadingLicenses: boolean;
	onSubmit: (request: TypesGen.CreateTrialLicenseRequest) => void;
	isSubmitting: boolean;
	error?: unknown;
}

export const PremiumPageView: FC<PremiumPageViewProps> = ({
	hasLicense,
	isTrial,
	canRequestTrial,
	trialDaysRemaining,
	isLoadingLicenses,
	onSubmit,
	isSubmitting,
	error,
}) => {
	let panel: React.ReactNode;
	if (hasLicense && !isTrial) {
		panel = <LicenseActivePanel />;
	} else if (hasLicense) {
		panel = (
			<TrialActivePanel
				daysRemaining={trialDaysRemaining}
				isLoading={isLoadingLicenses}
			/>
		);
	} else if (canRequestTrial) {
		panel = (
			<TrialRequestForm
				onSubmit={onSubmit}
				isSubmitting={isSubmitting}
				error={error}
			/>
		);
	} else {
		panel = (
			<div className="flex flex-col gap-2 items-start">
				<PaywallTitle>{PREMIUM_DEFAULT_HERO}</PaywallTitle>
				<PaywallGuidance className="mx-0">
					Contact your deployment administrator for Premium.
				</PaywallGuidance>
			</div>
		);
	}

	return (
		<div className="rounded-lg border border-solid border-border-default bg-surface-primary overflow-hidden">
			<div className="grid grid-cols-1 lg:grid-cols-2 min-h-[640px]">
				{/* Supergraphic is absolutely positioned at -z-10, so this parent must
				    establish a stacking context or it renders behind the page. */}
				<div className="relative isolate overflow-hidden hidden lg:block bg-surface-secondary">
					<Supergraphic className="bg-[length:auto_180%] bg-[position:20%_50%]" />
				</div>
				<div className="flex flex-col justify-center p-8 lg:p-12">{panel}</div>
			</div>
		</div>
	);
};
