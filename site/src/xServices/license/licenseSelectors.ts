import { State } from "xstate"
import { LicensePermission } from "../../api/types"
import { LicenseContext, LicenseEvent } from "./licenseXService"
type LicenseState = State<LicenseContext, LicenseEvent>

export const selectLicenseVisibility = (state: LicenseState): Record<LicensePermission, boolean> => {
  const features = state.context.licenseData.features
  const featureNames = Object.keys(features) as LicensePermission[]
  const visibilityPairs = featureNames.map((feature: LicensePermission) => {
    return [feature, features[feature].enabled]
  })
  return Object.fromEntries(visibilityPairs)
}

export const selectLicenseEntitlement = (state: LicenseState): Record<LicensePermission, boolean> => {
  const features = state.context.licenseData.features
  const featureNames = Object.keys(features) as LicensePermission[]
  const permissionPairs = featureNames.map((feature: LicensePermission) => {
    const { entitled, limit, actual } = features[feature]
    const limitCompliant = limit && actual && limit >= actual
    return [feature, entitled && limitCompliant]
  })
  return Object.fromEntries(permissionPairs)
}
