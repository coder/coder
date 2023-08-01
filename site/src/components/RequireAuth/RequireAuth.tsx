import { useAuth } from "components/AuthProvider/AuthProvider"
import { FC } from "react"
import { Outlet, Navigate, useLocation } from "react-router-dom"
import { embedRedirect } from "../../utils/redirect"
import { FullScreenLoader } from "../Loader/FullScreenLoader"
import { DashboardProvider } from "components/Dashboard/DashboardProvider"
import { ProxyProvider } from "contexts/ProxyContext"

export const RequireAuth: FC = () => {
  const [authState] = useAuth()
  const location = useLocation()
  const isHomePage = location.pathname === "/"
  const navigateTo = isHomePage ? "/login" : embedRedirect(location.pathname)

  if (authState.matches("signedOut")) {
    return <Navigate to={navigateTo} state={{ isRedirect: !isHomePage }} />
  } else if (authState.matches("configuringTheFirstUser")) {
    return <Navigate to="/setup" />
  } else if (
    authState.matches("loadingInitialAuthData") ||
    authState.matches("signingOut")
  ) {
    return <FullScreenLoader />
  } else {
    // Authenticated pages have access to some contexts for knowing enabled experiments
    // and where to route workspace connections.
    return (
      <DashboardProvider>
        <ProxyProvider>
          <Outlet />
        </ProxyProvider>
      </DashboardProvider>
    )
  }
}
