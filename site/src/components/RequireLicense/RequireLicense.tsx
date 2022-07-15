import { useSelector } from "@xstate/react"
import React, { useContext } from "react"
import { Navigate } from "react-router"
import { FullScreenLoader } from "../Loader/FullScreenLoader"
import { XServiceContext } from "../../xServices/StateContext"
import { selectLicenseVisibility } from "../../xServices/license/licenseSelectors"
import { LicensePermission } from "../../api/types"

export interface RequireLicenseProps {
  children: JSX.Element
  permissionRequired: LicensePermission
}

export const RequireLicense: React.FC<RequireLicenseProps> = ({ children, permissionRequired }) => {
  const xServices = useContext(XServiceContext)
  const visibility = useSelector(xServices.licenseXService, selectLicenseVisibility)
  if (!visibility) {
    return <FullScreenLoader />
  } else if (!visibility[permissionRequired]) {
    return <Navigate to="/not-found" />
  } else {
    return children
  }
}
