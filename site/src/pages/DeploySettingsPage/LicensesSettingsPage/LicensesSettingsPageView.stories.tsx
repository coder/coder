import { GetLicensesResponse } from "api/api"
import LicensesSettingsPageView from "./LicensesSettingsPageView"

export default {
  title: "pages/LicensesSettingsPage",
  component: LicensesSettingsPageView,
}

const licensesTest: GetLicensesResponse[] = [
  {
    id: 1,
    claims: {
      trial: false,
      all_features: true,
      version: 1,
      features: {},
      license_expires: 1682346425,
    },
  },
]

export const Default = {
  args: {
    showConfetti: false,
    isLoading: false,
    userLimitActual: 1,
    userLimitLimit: 10,
    licenses: licensesTest,
  },
}
