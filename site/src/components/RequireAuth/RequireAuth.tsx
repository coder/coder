import axios from "axios";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC, useEffect } from "react";
import { Outlet, Navigate, useLocation } from "react-router-dom";
import { embedRedirect } from "../../utils/redirect";
import { FullScreenLoader } from "../Loader/FullScreenLoader";
import { DashboardProvider } from "components/Dashboard/DashboardProvider";
import { ProxyProvider } from "contexts/ProxyContext";
import { isApiError } from "api/errors";

export const RequireAuth: FC = () => {
  const [authState, authSend] = useAuth();
  const location = useLocation();
  const isHomePage = location.pathname === "/";
  const navigateTo = isHomePage
    ? "/login"
    : embedRedirect(`${location.pathname}${location.search}`);

  useEffect(() => {
    const interceptorHandle = axios.interceptors.response.use(
      (okResponse) => okResponse,
      (error: unknown) => {
        // 401 Unauthorized
        // If we encountered an authentication error, then our token is probably
        // invalid and we should update the auth state to reflect that.
        if (isApiError(error) && error.response.status === 401) {
          authSend("SIGN_OUT");
        }

        // Otherwise, pass the response through so that it can be displayed in the UI
        return Promise.reject(error);
      },
    );

    return () => {
      axios.interceptors.response.eject(interceptorHandle);
    };
  }, [authSend]);

  if (authState.matches("signedOut")) {
    return <Navigate to={navigateTo} state={{ isRedirect: !isHomePage }} />;
  } else if (authState.matches("configuringTheFirstUser")) {
    return <Navigate to="/setup" />;
  } else if (
    authState.matches("loadingInitialAuthData") ||
    authState.matches("signingOut")
  ) {
    return <FullScreenLoader />;
  } else {
    // Authenticated pages have access to some contexts for knowing enabled experiments
    // and where to route workspace connections.
    return (
      <DashboardProvider>
        <ProxyProvider>
          <Outlet />
        </ProxyProvider>
      </DashboardProvider>
    );
  }
};
