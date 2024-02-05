import { chromatic } from "testHelpers/chromatic";
import { MockLicenseResponse } from "testHelpers/entities";
import LicensesSettingsPageView from "./LicensesSettingsPageView";

export default {
  title: "pages/DeploySettingsPage/LicensesSettingsPageView",
  parameters: { chromatic },
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
