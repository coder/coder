import axios from "axios";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC, useEffect } from "react";
import { Outlet, Navigate, useLocation } from "react-router-dom";
import { embedRedirect } from "utils/redirect";
import { FullScreenLoader } from "../Loader/FullScreenLoader";
import { DashboardProvider } from "components/Dashboard/DashboardProvider";
import { ProxyProvider } from "contexts/ProxyContext";
import { isApiError } from "api/errors";

export const RequireAuth: FC = () => {
  const { signOut, isSigningOut, isSignedOut } = useAuth();
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
          signOut();
        }

        // Otherwise, pass the response through so that it can be displayed in the UI
        return Promise.reject(error);
      },
    );

    return () => {
      axios.interceptors.response.eject(interceptorHandle);
    };
  }, [signOut]);

  if (isSignedOut) {
    return (
      <Navigate to={navigateTo} state={{ isRedirect: !isHomePage }} replace />
    );
  } else if (isSigningOut) {
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
