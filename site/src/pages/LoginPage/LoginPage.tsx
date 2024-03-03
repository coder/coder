import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "contexts/auth/useAuth";
import { getApplicationName } from "utils/appearance";
import { retrieveRedirect } from "utils/redirect";
import { LoginPageView } from "./LoginPageView";

export const LoginPage: FC = () => {
  const location = useLocation();
  const {
    isLoading,
    isSignedIn,
    isConfiguringTheFirstUser,
    signIn,
    isSigningIn,
    authMethods,
    signInError,
  } = useAuth();
  const redirectTo = retrieveRedirect(location.search);
  const applicationName = getApplicationName();
  const navigate = useNavigate();

  if (isSignedIn) {
    // If the redirect is going to a workspace application, and we
    // are missing authentication, then we need to change the href location
    // to trigger a HTTP request. This allows the BE to generate the auth
    // cookie required.  Similarly for the OAuth2 exchange as the authorization
    // page is served by the backend.
    // If no redirect is present, then ignore this branched logic.
    if (redirectTo !== "" && redirectTo !== "/") {
      try {
        // This catches any absolute redirects. Relative redirects
        // will fail the try/catch. Subdomain apps are absolute redirects.
        const redirectURL = new URL(redirectTo);
        if (redirectURL.host !== window.location.host) {
          window.location.href = redirectTo;
          return null;
        }
      } catch {
        // Do nothing
      }
      // Path based apps and OAuth2.
      if (redirectTo.includes("/apps/") || redirectTo.includes("/oauth2/")) {
        window.location.href = redirectTo;
        return null;
      }
    }

    return <Navigate to={redirectTo} replace />;
  }

  if (isConfiguringTheFirstUser) {
    return <Navigate to="/setup" replace />;
  }

  return (
    <>
      <Helmet>
        <title>Sign in to {applicationName}</title>
      </Helmet>
      <LoginPageView
        authMethods={authMethods}
        error={signInError}
        isLoading={isLoading}
        isSigningIn={isSigningIn}
        onSignIn={async ({ email, password }) => {
          await signIn(email, password);
          navigate("/");
        }}
      />
    </>
  );
};

export default LoginPage;
