import { GetLicensesResponse } from "api/api"
import LicensesSettingsPageView from "./LicensesSettingsPageView"

export default {
  title: "pages/LicensesSettingsPageView",
  component: LicensesSettingsPageView,
}

const licensesTest: GetLicensesResponse[] = [
  {
    id: 1,
    uploaded_at: "1682346425",
    expires_at: "1682346425",
    uuid: "1",
    claims: {
      trial: false,
      all_features: true,
      version: 1,
      features: {},
      license_expires: 1682346425,
    },
  },
]

const defaultArgs = {
  showConfetti: false,
  isLoading: false,
  userLimitActual: 1,
  userLimitLimit: 10,
  licenses: licensesTest,
}

export const Default = {
  args: defaultArgs,
}

export const Empty = {
  args: {
    ...defaultArgs,
    licenses: null,
  },
}
