import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { PaywallGuidance } from "#/components/Paywall/Paywall";
import { Supergraphic } from "#/components/Supergraphic/Supergraphic";
import { LicenseActivePanel } from "./LicenseActivePanel";
import { TrialActivePanel } from "./TrialActivePanel";
import { TrialRequestForm } from "./TrialRequestForm";

type PremiumPageViewProps = {
	hasLicense: boolean;
	isTrial: boolean;
	/** Whether the viewer may request a trial for this deployment. */
	canRequestTrial: boolean;
	trialDaysRemaining: number | undefined;
	onSubmit: (request: TypesGen.CreateTrialLicenseRequest) => void;
	isSubmitting: boolean;
	error?: unknown;
};

export const PremiumPageView: FC<PremiumPageViewProps> = ({
	hasLicense,
	isTrial,
	canRequestTrial,
	trialDaysRemaining,
	onSubmit,
	isSubmitting,
	error,
}) => {
	// An installed license has nothing to sell, so both license states collapse
	// the two-column pitch into a single banner.
	if (hasLicense) {
		return (
			<div className="relative isolate overflow-hidden rounded-lg py-12 mb-8 border border-solid bg-surface-secondary flex items-center justify-center">
				<Supergraphic className="bg-[position:20%_20%]" />
				{isTrial ? (
					<TrialActivePanel daysRemaining={trialDaysRemaining} />
				) : (
					<LicenseActivePanel />
				)}
			</div>
		);
	}

	return (
		<div className="rounded-lg border border-solid border-border-default bg-surface-primary overflow-hidden">
			<div className="grid grid-cols-1 lg:grid-cols-2 min-h-[640px]">
				<div className="relative isolate overflow-hidden hidden lg:flex flex-col p-12 bg-surface-secondary">
					<Supergraphic className="bg-[position:20%_20%] bg-[length:110%_125%] -scale-x-100" />
					<h2 className="self-start m-0 pt-24 max-w-md text-3xl font-semibold text-content-primary text-balance">
						Start an unlimited 30-day Coder trial
					</h2>
					<p className="self-start m-0 max-w-sm pt-6 text-sm text-content-primary">
						Unlock unlimited Coder Agents usage and enterprise AI governance.
						Control what agents can access, manage who can use templates and
						infrastructure, and run your self-hosted development platform with
						role-based access controls and high availability.
					</p>
				</div>
				<div className="flex flex-col justify-center p-8 lg:p-12 bg-surface-secondary">
					{canRequestTrial ? (
						<TrialRequestForm
							onSubmit={onSubmit}
							isSubmitting={isSubmitting}
							error={error}
						/>
					) : (
						<PaywallGuidance className="mx-0">
							Contact your deployment administrator for Premium.
						</PaywallGuidance>
					)}
				</div>
			</div>
		</div>
	);
};
