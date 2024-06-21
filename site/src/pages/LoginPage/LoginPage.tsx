import { useEffect, type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { buildInfo } from "api/queries/buildInfo";
import { authMethods } from "api/queries/users";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { getApplicationName } from "utils/appearance";
import { retrieveRedirect } from "utils/redirect";
import { sendDeploymentEvent } from "utils/telemetry";
import { LoginPageView } from "./LoginPageView";

export const LoginPage: FC = () => {
  const location = useLocation();
  const {
    isLoading,
    isSignedIn,
    isConfiguringTheFirstUser,
    signIn,
    isSigningIn,
    signInError,
    user,
  } = useAuthContext();
  const authMethodsQuery = useQuery(authMethods());
  const redirectTo = retrieveRedirect(location.search);
  const applicationName = getApplicationName();
  const navigate = useNavigate();
  const { metadata } = useEmbeddedMetadata();
  const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));

  useEffect(() => {
    if (!buildInfoQuery.data || isSignedIn) {
      // isSignedIn already tracks with window.href!
      return;
    }
    // This uses `navigator.sendBeacon`, so navigating away will not prevent it!
    sendDeploymentEvent(buildInfoQuery.data, {
      type: "deployment_login",
      user_id: user?.id,
    });
  }, [isSignedIn, buildInfoQuery.data, user?.id]);

  if (isSignedIn) {
    if (buildInfoQuery.data) {
      // This uses `navigator.sendBeacon`, so window.href
      // will not stop the request from being sent!
      sendDeploymentEvent(buildInfoQuery.data, {
        type: "deployment_login",
        user_id: user?.id,
      });
    }

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
        authMethods={authMethodsQuery.data}
        error={signInError}
        isLoading={isLoading || authMethodsQuery.isLoading}
        buildInfo={buildInfoQuery.data}
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
