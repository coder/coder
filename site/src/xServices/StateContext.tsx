import { useInterpret } from "@xstate/react"
import { createContext, FC, ReactNode } from "react"
import { ActorRefFrom } from "xstate"
import { authMachine } from "./auth/authXService"
import { buildInfoMachine } from "./buildInfo/buildInfoXService"
import { updateCheckMachine } from "./updateCheck/updateCheckXService"
import { deploymentConfigMachine } from "./deploymentConfig/deploymentConfigMachine"
import { entitlementsMachine } from "./entitlements/entitlementsXService"
import { siteRolesMachine } from "./roles/siteRolesXService"
import { serviceBannerMachine } from "./serviceBanner/serviceBannerXService"
import { useNavigate } from "react-router-dom"
import * as API from "api/api"

interface XServiceContextType {
  authXService: ActorRefFrom<typeof authMachine>
  buildInfoXService: ActorRefFrom<typeof buildInfoMachine>
  entitlementsXService: ActorRefFrom<typeof entitlementsMachine>
  serviceBannerXService: ActorRefFrom<typeof serviceBannerMachine>
  siteRolesXService: ActorRefFrom<typeof siteRolesMachine>
  // Since the info here is used by multiple deployment settings page and we don't want to refetch them every time
  deploymentConfigXService: ActorRefFrom<typeof deploymentConfigMachine>
  updateCheckXService: ActorRefFrom<typeof updateCheckMachine>
}

/**
 * Consuming this Context will not automatically cause rerenders because
 * the xServices in it are static references.
 *
 * To use one of the xServices, `useActor` will access all its state
 * (causing re-renders for any changes to that one xService) and
 * `useSelector` will access just one piece of state.
 */
export const XServiceContext = createContext({} as XServiceContextType)

export const XServiceProvider: FC<{ children: ReactNode }> = ({ children }) => {
  const navigate = useNavigate()

  return (
    <XServiceContext.Provider
      value={{
        authXService: useInterpret(authMachine, {
          actions: {
            redirectAfterLogout: async () => {
              const appHost = await API.getApplicationsHost()
              if (appHost.host) {
                const redirect_uri = encodeURIComponent(window.location.href)
                const uri = `${
                  window.location.protocol
                }//${appHost.host.replace(
                  "*",
                  "coder-logout",
                )}/api/logout?redirect_uri=${redirect_uri}`
                // window.location.replace(uri) // this is the culprit
                navigate(uri)
              }
            },
          },
        }),
        buildInfoXService: useInterpret(buildInfoMachine),
        entitlementsXService: useInterpret(entitlementsMachine),
        serviceBannerXService: useInterpret(serviceBannerMachine),
        siteRolesXService: useInterpret(siteRolesMachine),
        deploymentConfigXService: useInterpret(deploymentConfigMachine),
        updateCheckXService: useInterpret(updateCheckMachine),
      }}
    >
      {children}
    </XServiceContext.Provider>
  )
}
