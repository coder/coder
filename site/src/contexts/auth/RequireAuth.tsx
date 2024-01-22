import axios from "axios";
import { type FC, useEffect } from "react";
import { Outlet, Navigate, useLocation } from "react-router-dom";
import { embedRedirect } from "utils/redirect";
import { isApiError } from "api/errors";
import { ProxyProvider } from "contexts/ProxyContext";
import { DashboardProvider } from "components/Dashboard/DashboardProvider";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import { useAuth } from "./useAuth";

export const RequireAuth: FC = () => {
  const { signOut, isSigningOut, isSignedOut, isSignedIn, isLoading } =
    useAuth();
  const location = useLocation();
  const isHomePage = location.pathname === "/";
  const navigateTo = isHomePage
    ? "/login"
    : embedRedirect(`${location.pathname}${location.search}`);

  useEffect(() => {
    if (isLoading || isSigningOut || !isSignedIn) {
      return;
    }

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
  }, [isLoading, isSigningOut, isSignedIn, signOut]);

  if (isLoading || isSigningOut) {
    return <FullScreenLoader />;
  }

  if (isSignedOut) {
    return (
      <Navigate to={navigateTo} state={{ isRedirect: !isHomePage }} replace />
    );
  }

  // Authenticated pages have access to some contexts for knowing enabled experiments
  // and where to route workspace connections.
  return (
    <DashboardProvider>
      <ProxyProvider>
        <Outlet />
      </ProxyProvider>
    </DashboardProvider>
  );
};
