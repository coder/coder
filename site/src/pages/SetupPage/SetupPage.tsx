import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { SetupPageView } from "./SetupPageView";
import { Navigate } from "react-router-dom";
import { useMutation } from "react-query";
import { createFirstUser } from "api/queries/users";

export const SetupPage: FC = () => {
  const { signIn, isLoading, isConfiguringTheFirstUser, isSignedIn } =
    useAuth();
  const createFirstUserMutation = useMutation(createFirstUser());
  const setupIsComplete = !isLoading && !isConfiguringTheFirstUser;

  // If the user is logged in, navigate to the app
  if (isSignedIn) {
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
          signIn(firstUser.email, firstUser.password);
        }}
      />
    </>
  );
};
