import dayjs from "dayjs";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { createTrialLicense, licenses } from "#/api/queries/licenses";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { DATABASE_DOCS_LINK } from "#/modules/licenses/trialLicense";
import { pageTitle } from "#/utils/page";
import { PremiumPageView } from "./PremiumPageView";

const PremiumPage: FC = () => {
	const { entitlements } = useDashboard();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const hasLicense = entitlements.has_license;
	const isTrial = entitlements.trial;

	// Only the trial panel needs license claims; other states render entitlements
	const licensesQuery = useQuery({
		...licenses(),
		enabled: hasLicense && isTrial,
	});
	const trialMutation = useMutation(createTrialLicense(queryClient));

	const expiresAt = licensesQuery.data?.find((license) => license.claims.trial)
		?.claims.license_expires;
	const trialDaysRemaining = Number.isFinite(expiresAt)
		? dayjs.unix(Number(expiresAt)).diff(dayjs(), "day")
		: undefined;

	return (
		<>
			<title>{pageTitle("Premium")}</title>

			<SettingsHeader
				actions={
					<SettingsHeaderDocsLink
						href={DATABASE_DOCS_LINK}
					></SettingsHeaderDocsLink>
				}
			>
				<SettingsHeaderTitle>Premium Trial</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					For enterprises ready to achieve world-class security, scalability,
					and developer experience.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<PremiumPageView
				hasLicense={hasLicense}
				isTrial={isTrial}
				canRequestTrial={permissions.viewAllLicenses}
				trialDaysRemaining={trialDaysRemaining}
				isLoadingLicenses={licensesQuery.isLoading}
				isSubmitting={trialMutation.isPending}
				error={trialMutation.error}
				onSubmit={(request) => {
					trialMutation.mutate(request, {
						onSuccess: () => {
							navigate("/deployment/licenses?success=true");
						},
						onError: (error) => {
							toast.error(
								getErrorMessage(error, "Failed to request a trial license."),
								{ description: getErrorDetail(error) },
							);
						},
					});
				}}
			/>
		</>
	);
};

export default PremiumPage;
