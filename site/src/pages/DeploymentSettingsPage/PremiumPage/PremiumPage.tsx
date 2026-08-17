import dayjs from "dayjs";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { createTrialLicense, licenses } from "#/api/queries/licenses";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { PremiumPageView } from "./PremiumPageView";
import { LICENSES_PAGE_PATH } from "./TrialActivePanel";

const PremiumPage: FC = () => {
	const { entitlements } = useDashboard();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const navigate = useNavigate();

	const hasLicense = entitlements.has_license;
	const isTrial = entitlements.trial;

	// Only the trial panel needs license claims; every other state renders from
	// entitlements alone.
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
							navigate(`${LICENSES_PAGE_PATH}?success=true`);
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
