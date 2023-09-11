import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getLicenses, removeLicense } from "api/api";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useSearchParams } from "react-router-dom";
import useToggle from "react-use/lib/useToggle";
import { pageTitle } from "utils/page";
import LicensesSettingsPageView from "./LicensesSettingsPageView";
import { getErrorMessage } from "api/errors";
import {
  useEntitlements,
  useRefreshEntitlements,
} from "api/queries/entitlements";

const LicensesSettingsPage: FC = () => {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const success = searchParams.get("success");
  const [confettiOn, toggleConfettiOn] = useToggle(false);
  const entitlements = useEntitlements();
  const refreshEntitlements = useRefreshEntitlements({
    onSuccess: () => {
      displaySuccess("Successfully refreshed licenses");
    },
    onError: (error) => {
      displayError(getErrorMessage(error, "Failed to refresh entitlements"));
    },
  });

  useEffect(() => {
    if (entitlements.error) {
      displayError(
        getErrorMessage(entitlements.error, "Failed to fetch entitlements"),
      );
    }
  }, [entitlements.error]);

  const { mutate: removeLicenseApi, isLoading: isRemovingLicense } =
    useMutation(removeLicense, {
      onSuccess: () => {
        displaySuccess("Successfully removed license");
        void queryClient.invalidateQueries(["licenses"]);
      },
      onError: () => {
        displayError("Failed to remove license");
      },
    });

  const { data: licenses, isLoading } = useQuery({
    queryKey: ["licenses"],
    queryFn: () => getLicenses(),
  });

  useEffect(() => {
    if (success) {
      toggleConfettiOn();
      const timeout = setTimeout(() => {
        toggleConfettiOn(false);
        setSearchParams();
      }, 2000);
      return () => clearTimeout(timeout);
    }
  }, [setSearchParams, success, toggleConfettiOn]);

  return (
    <>
      <Helmet>
        <title>{pageTitle("License Settings")}</title>
      </Helmet>
      <LicensesSettingsPageView
        showConfetti={confettiOn}
        isLoading={isLoading}
        isRefreshing={refreshEntitlements.isLoading}
        userLimitActual={entitlements.data?.features.user_limit.actual}
        userLimitLimit={entitlements.data?.features.user_limit.limit}
        licenses={licenses}
        isRemovingLicense={isRemovingLicense}
        removeLicense={(licenseId: number) => removeLicenseApi(licenseId)}
        refreshEntitlements={() => {
          refreshEntitlements.mutate();
        }}
      />
    </>
  );
};

export default LicensesSettingsPage;
