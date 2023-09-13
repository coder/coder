import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMachine } from "@xstate/react";
import { getLicenses, removeLicense } from "api/api";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useSearchParams } from "react-router-dom";
import useToggle from "react-use/lib/useToggle";
import { pageTitle } from "utils/page";
import { entitlementsMachine } from "xServices/entitlements/entitlementsXService";
import LicensesSettingsPageView from "./LicensesSettingsPageView";
import { getErrorMessage } from "api/errors";

const LicensesSettingsPage: FC = () => {
  const queryClient = useQueryClient();
  const [entitlementsState, sendEvent] = useMachine(entitlementsMachine);
  const { entitlements, getEntitlementsError } = entitlementsState.context;
  const [searchParams, setSearchParams] = useSearchParams();
  const success = searchParams.get("success");
  const [confettiOn, toggleConfettiOn] = useToggle(false);
  if (getEntitlementsError) {
    displayError(
      getErrorMessage(getEntitlementsError, "Failed to fetch entitlements"),
    );
  }

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
        userLimitActual={entitlements?.features.user_limit.actual}
        userLimitLimit={entitlements?.features.user_limit.limit}
        licenses={licenses}
        isRemovingLicense={isRemovingLicense}
        removeLicense={(licenseId: number) => removeLicenseApi(licenseId)}
        refreshEntitlements={() => {
          const x = sendEvent("REFRESH");
          return !x.context.getEntitlementsError;
        }}
      />
    </>
  );
};

export default LicensesSettingsPage;
