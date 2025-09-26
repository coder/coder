import { API } from "api/api";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import type { FC } from "react";
import { useMutation } from "react-query";
import { useNavigate } from "react-router";
import { pageTitle } from "utils/page";
import { AddNewLicensePageView } from "./AddNewLicensePageView";

const AddNewLicensePage: FC = () => {
	const navigate = useNavigate();

	const {
		mutate: saveLicenseKeyApi,
		isPending: isCreating,
		error: savingLicenseError,
	} = useMutation({
		mutationFn: API.createLicense,
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
			<title>{pageTitle("License Settings")}</title>

			<AddNewLicensePageView
				isSavingLicense={isCreating}
				savingLicenseError={savingLicenseError}
				onSaveLicenseKey={saveLicenseKey}
			/>
		</>
	);
};

export default AddNewLicensePage;
