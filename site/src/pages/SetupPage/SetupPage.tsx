import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { SetupPageView } from "./SetupPageView";
import { Navigate } from "react-router-dom";
import { useMutation } from "react-query";
import { createFirstUser } from "api/queries/users";

export const SetupPage: FC = () => {
  const [authState, authSend] = useAuth();
  const createFirstUserMutation = useMutation(createFirstUser());
  const userIsSignedIn = authState.matches("signedIn");
  const setupIsComplete =
    !authState.matches("loadingInitialAuthData") &&
    !authState.matches("configuringTheFirstUser");

  // If the user is logged in, navigate to the app
  if (userIsSignedIn) {
    return <Navigate to="/" state={{ isRedirect: true }} />;
  }

  // If we've already completed setup, navigate to the login page
  if (setupIsComplete) {
    return <Navigate to="/login" state={{ isRedirect: true }} />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Set up your account")}</title>
      </Helmet>
      <SetupPageView
        isLoading={createFirstUserMutation.isLoading}
        error={createFirstUserMutation.error}
        onSubmit={async (firstUser) => {
          await createFirstUserMutation.mutateAsync(firstUser);
          authSend({
            type: "SIGN_IN",
            email: firstUser.email,
            password: firstUser.password,
          });
        }}
      />
    </>
  );
};
