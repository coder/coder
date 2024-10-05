import { AddNewLicensePageView } from "./AddNewLicensePageView";

export default {
	title: "pages/DeploymentSettingsPage/AddNewLicensePageView",
	component: AddNewLicensePageView,
};

export const Default = {
	args: {
		isSavingLicense: false,
		didSavingFailed: false,
	},
};
