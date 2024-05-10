import { type FC, useEffect } from "react";
import { Outlet, Navigate, useLocation } from "react-router-dom";
import { axiosInstance } from "api/api";
import { isApiError } from "api/errors";
import { Loader } from "components/Loader/Loader";
import { ProxyProvider } from "contexts/ProxyContext";
import { DashboardProvider } from "modules/dashboard/DashboardProvider";
import { embedRedirect } from "utils/redirect";
import { type AuthContextValue, useAuthContext } from "./AuthProvider";

export const RequireAuth: FC = () => {
  const location = useLocation();
  const { signOut, isSigningOut, isSignedOut, isSignedIn, isLoading } =
    useAuthContext();

  useEffect(() => {
    if (isLoading || isSigningOut || !isSignedIn) {
      return;
    }

    const interceptorHandle = axiosInstance.interceptors.response.use(
      (okResponse) => okResponse,
      (error: unknown) => {
        // 401 Unauthorized
        // If we encountered an authentication error, then our token is probably
        // invalid and we should update the auth state to reflect that.
        if (isApiError(error) && error.response.status === 401) {
          signOut();
        }

        // Otherwise, pass the response through so that it can be displayed in
        // the UI
        return Promise.reject(error);
      },
    );

    return () => {
      axiosInstance.interceptors.response.eject(interceptorHandle);
    };
  }, [isLoading, isSigningOut, isSignedIn, signOut]);

  if (isLoading || isSigningOut) {
    return <Loader fullscreen />;
  }

  if (isSignedOut) {
    const isHomePage = location.pathname === "/";
    const navigateTo = isHomePage
      ? "/login"
      : embedRedirect(`${location.pathname}${location.search}`);

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

// We can do some TS magic here but I would rather to be explicit on what
// values are not undefined when authenticated
type NonNullableAuth = AuthContextValue & {
  user: Exclude<AuthContextValue["user"], undefined>;
  permissions: Exclude<AuthContextValue["permissions"], undefined>;
  organizationId: Exclude<AuthContextValue["organizationId"], undefined>;
};

export const useAuthenticated = (): NonNullableAuth => {
  const auth = useAuthContext();

  if (!auth.user) {
    throw new Error("User is not authenticated.");
  }

  if (!auth.permissions) {
    throw new Error("Permissions are not available.");
  }

  return auth as NonNullableAuth;
};
