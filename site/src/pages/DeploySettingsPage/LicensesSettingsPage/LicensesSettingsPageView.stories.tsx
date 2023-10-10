import LicensesSettingsPageView from "./LicensesSettingsPageView";
import { MockLicenseResponse } from "testHelpers/entities";

export default {
  title: "pages/DeploySettingsPage/LicensesSettingsPageView",
  component: LicensesSettingsPageView,
};

const defaultArgs = {
  showConfetti: false,
  isLoading: false,
  userLimitActual: 1,
  userLimitLimit: 10,
  licenses: MockLicenseResponse,
};

export const Default = {
  args: defaultArgs,
};

export const Empty = {
  args: {
    ...defaultArgs,
    licenses: null,
  },
};
