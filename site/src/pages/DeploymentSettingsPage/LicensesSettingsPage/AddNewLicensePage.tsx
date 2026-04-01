import { API } from "api/api";
import { getErrorDetail } from "api/errors";
import type { FC } from "react";
import { useMutation } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
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
			toast.success("You have successfully added a license.");
			navigate("/deployment/licenses?success=true");
		},
		onError: (error) =>
			toast.error("Failed to save license key.", {
				description: getErrorDetail(error),
			}),
	});

	function saveLicenseKey(licenseKey: string) {
		saveLicenseKeyApi(
			{ license: licenseKey },
			{
				onSuccess: () => {
					toast.success("You have successfully added a license.");
					navigate("/deployment/licenses?success=true");
				},
				onError: (error) =>
					toast.error("Failed to save license key.", {
						description: getErrorDetail(error),
					}),
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
