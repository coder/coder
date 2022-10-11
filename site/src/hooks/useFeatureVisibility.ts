import { useSelector } from "@xstate/react"
import { useContext } from "react"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { XServiceContext } from "xServices/StateContext"

export const useFeatureVisibility = (): Record<string, boolean> => {
  const xServices = useContext(XServiceContext)
  return useSelector(xServices.entitlementsXService, selectFeatureVisibility)
}
