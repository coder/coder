import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useTranslation } from "react-i18next";
import { Navigate, useLocation } from "react-router-dom";
import { retrieveRedirect } from "../../utils/redirect";
import { LoginPageView } from "./LoginPageView";

export const LoginPage: FC = () => {
  const location = useLocation();
  const [authState, authSend] = useAuth();
  const redirectTo = retrieveRedirect(location.search);
  const commonTranslation = useTranslation("common");
  const loginPageTranslation = useTranslation("loginPage");

  if (authState.matches("signedIn")) {
    return <Navigate to={redirectTo} replace />;
  } else if (authState.matches("configuringTheFirstUser")) {
    return <Navigate to="/setup" />;
  } else {
    return (
      <>
        <Helmet>
          <title>
            {loginPageTranslation.t("signInTo")} {commonTranslation.t("coder")}
          </title>
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
