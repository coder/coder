import { useSelector } from "@xstate/react"
import { FeatureNames } from "api/types"
import { useContext } from "react"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "xServices/StateContext"

export const useFeatureVisibility = (): Record<FeatureNames, boolean> => {
  const xServices = useContext(XServiceContext)
  return useSelector(xServices.entitlementsXService, selectFeatureVisibility)
}
