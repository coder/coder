import { useAuth } from "components/AuthProvider/AuthProvider"
import { FC } from "react"
import { Navigate, useLocation } from "react-router"
import { Outlet } from "react-router-dom"
import { embedRedirect } from "../../util/redirect"
import { FullScreenLoader } from "../Loader/FullScreenLoader"

export const RequireAuth: FC = () => {
  const [authState] = useAuth()
  const location = useLocation()
  const isHomePage = location.pathname === "/"
  const navigateTo = isHomePage ? "/login" : embedRedirect(location.pathname)

  if (authState.matches("signedOut")) {
    return <Navigate to={navigateTo} state={{ isRedirect: !isHomePage }} />
  } else if (authState.matches("waitingForTheFirstUser")) {
    return <Navigate to="/setup" />
  } else if (authState.hasTag("loading")) {
    return <FullScreenLoader />
  } else {
    return <Outlet />
  }
}
