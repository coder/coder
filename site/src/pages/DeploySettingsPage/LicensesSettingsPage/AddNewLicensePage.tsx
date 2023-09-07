import { useMutation } from "@tanstack/react-query";
import { createLicense } from "api/api";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { FC } from "react";
import { useNavigate } from "react-router-dom";
import { AddNewLicensePageView } from "./AddNewLicensePageView";
import { pageTitle } from "utils/page";
import { Helmet } from "react-helmet-async";

const AddNewLicensePage: FC = () => {
  const navigate = useNavigate();

  const {
    mutate: saveLicenseKeyApi,
    isLoading: isCreating,
    error: savingLicenseError,
  } = useMutation(createLicense, {
    onSuccess: () => {
      displaySuccess("You have successfully added a license");
      navigate("/deployment/licenses?success=true");
    },
    onError: () => displayError("Failed to save license key"),
  });

  function saveLicenseKey(licenseKey: string) {
    saveLicenseKeyApi(
      { license: licenseKey },
      {
        onSuccess: () => {
          displaySuccess("You have successfully added a license");
          navigate("/deployment/licenses?success=true");
        },
        onError: () => displayError("Failed to save license key"),
      },
    );
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("License Settings")}</title>
      </Helmet>

      <AddNewLicensePageView
        isSavingLicense={isCreating}
        savingLicenseError={savingLicenseError}
        onSaveLicenseKey={saveLicenseKey}
      />
    </>
  );
};

export default AddNewLicensePage;
