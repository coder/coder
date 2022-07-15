import { useActor } from "@xstate/react"
import { useContext, useEffect } from "react"
import { XServiceContext } from "../../xServices/StateContext"

export const LicenseBanner: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [licenseState, licenseSend] = useActor(xServices.licenseXService)
  const warnings = licenseState.context.licenseData.warnings

  /** Gets license data on app mount because LicenseBanner is mounted in App */
  useEffect(() => {
    licenseSend("GET_LICENSE_DATA")
  }, [licenseSend])

  if (warnings) {
    return <div>{warnings.map((warning, i) => <p key={`${i}`}>{warning}</p>)}</div>
  } else {
    return null
  }
}
