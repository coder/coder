import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { entitlements, refreshEntitlements } from "api/queries/entitlements";
import { insightsUserStatusCounts } from "api/queries/insights";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { toast } from "sonner";
import { pageTitle } from "utils/page";
import LicensesSettingsPageView from "./LicensesSettingsPageView";

const LicensesSettingsPage: FC = () => {
	const queryClient = useQueryClient();
	const [searchParams, setSearchParams] = useSearchParams();
	const success = searchParams.get("success");
	const [confettiOn, setConfettiOn] = useState(false);

	const { metadata } = useEmbeddedMetadata();
	const entitlementsQuery = useQuery(entitlements(metadata.entitlements));

	const { data: userStatusCount } = useQuery(insightsUserStatusCounts());

	const refreshEntitlementsMutation = useMutation(
		refreshEntitlements(queryClient),
	);

	useEffect(() => {
		if (entitlementsQuery.error) {
			toast.error(
				getErrorMessage(
					entitlementsQuery.error,
					"Failed to fetch entitlements.",
				),
				{
					description: getErrorDetail(entitlementsQuery.error),
				},
			);
		}
	}, [entitlementsQuery.error]);

	const { mutate: removeLicenseApi, isPending: isRemovingLicense } =
		useMutation({
			mutationFn: API.removeLicense,
			onSuccess: () => {
				toast.success("Successfully removed license.");
				void queryClient.invalidateQueries({ queryKey: ["licenses"] });
			},
			onError: (error) => {
				toast.error("Failed to remove license.", {
					description: getErrorDetail(error),
				});
			},
		});

	const { data: licenses, isLoading } = useQuery({
		queryKey: ["licenses"],
		queryFn: () => API.getLicenses(),
	});

	useEffect(() => {
		if (!success) {
			return;
		}

		setConfettiOn(true);
		const timeout = setTimeout(() => {
			setConfettiOn(false);
			setSearchParams();
		}, 2000);

		return () => {
			clearTimeout(timeout);
		};
	}, [setSearchParams, success]);

	return (
		<>
			<title>{pageTitle("License Settings")}</title>

			<LicensesSettingsPageView
				showConfetti={confettiOn}
				isLoading={isLoading}
				isRefreshing={refreshEntitlementsMutation.isPending}
				userLimitActual={entitlementsQuery.data?.features.user_limit?.actual}
				userLimitLimit={entitlementsQuery.data?.features.user_limit?.limit}
				licenses={licenses}
				isRemovingLicense={isRemovingLicense}
				removeLicense={(licenseId: number) => removeLicenseApi(licenseId)}
				activeUsers={userStatusCount?.active}
				managedAgentFeature={
					entitlementsQuery.data?.features.managed_agent_limit
				}
				aiGovernanceUserFeature={
					entitlementsQuery.data?.features.ai_governance_user_limit
				}
				refreshEntitlements={async () => {
					try {
						await refreshEntitlementsMutation.mutateAsync();
						toast.success("Successfully removed license.");
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to remove license."), {
							description: getErrorDetail(error),
						});
					}
				}}
			/>
		</>
	);
};

export default LicensesSettingsPage;
