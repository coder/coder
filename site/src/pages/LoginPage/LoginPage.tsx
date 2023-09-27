import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { Navigate, useLocation } from "react-router-dom";
import { retrieveRedirect } from "utils/redirect";
import { LoginPageView } from "./LoginPageView";

export const LoginPage: FC = () => {
  const location = useLocation();
  const [authState, authSend] = useAuth();
  const redirectTo = retrieveRedirect(location.search);

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />;
  } else if (authState.matches("configuringTheFirstUser")) {
    return <Navigate to="/setup" />;
  } else {
    return (
      <>
        <Helmet>
          <title>Sign in to Coder</title>
        </Helmet>
        <LoginPageView
          context={authState.context}
          isLoading={authState.matches("loadingInitialAuthData")}
          isSigningIn={authState.matches("signingIn")}
          onSignIn={({ email, password }) => {
            authSend({ type: "SIGN_IN", email, password });
          }}
        />
      </>
    );
  }
};

export default LoginPage;
