import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { Navigate, useLocation } from "react-router-dom";
import { retrieveRedirect } from "utils/redirect";
import { LoginPageView } from "./LoginPageView";
import { getApplicationName } from "utils/appearance";

export const LoginPage: FC = () => {
  const location = useLocation();
  const {
    isSignedIn,
    isLoading,
    isConfiguringTheFirstUser,
    signIn,
    isSigningIn,
    authMethods,
    signInError,
  } = useAuth();
  const redirectTo = retrieveRedirect(location.search);
  const applicationName = getApplicationName();

  if (isSignedIn) {
    return <Navigate to={redirectTo} replace />;
  } else if (isConfiguringTheFirstUser) {
    return <Navigate to="/setup" />;
  } else {
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
          onSignIn={({ email, password }) => {
            signIn(email, password);
          }}
        />
      </>
    );
  }
};

export default LoginPage;
