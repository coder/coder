import { useSelector } from "@xstate/react"
import { FeatureName } from "api/typesGenerated"
import { useContext } from "react"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "xServices/StateContext"

export const useFeatureVisibility = (): Record<FeatureName, boolean> => {
  const xServices = useContext(XServiceContext)
  return useSelector(xServices.entitlementsXService, selectFeatureVisibility)
}
