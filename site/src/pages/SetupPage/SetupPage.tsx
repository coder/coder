import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation } from "react-query";
import { Navigate, useNavigate } from "react-router-dom";
import { createFirstUser } from "api/queries/users";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import { useAuth } from "contexts/auth/useAuth";
import { pageTitle } from "utils/page";
import { SetupPageView } from "./SetupPageView";

export const SetupPage: FC = () => {
  const {
    isLoading,
    signIn,
    isConfiguringTheFirstUser,
    isSignedIn,
    isSigningIn,
  } = useAuth();
  const createFirstUserMutation = useMutation(createFirstUser());
  const setupIsComplete = !isConfiguringTheFirstUser;
  const navigate = useNavigate();

  if (isLoading) {
    return <FullScreenLoader />;
  }

  // If the user is logged in, navigate to the app
  if (isSignedIn) {
    return <Navigate to="/" state={{ isRedirect: true }} replace />;
  }

  // If we've already completed setup, navigate to the login page
  if (setupIsComplete) {
    return <Navigate to="/login" state={{ isRedirect: true }} replace />;
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle("Set up your account")}</title>
      </Helmet>
      <SetupPageView
        isLoading={isSigningIn || createFirstUserMutation.isLoading}
        error={createFirstUserMutation.error}
        onSubmit={async (firstUser) => {
          await createFirstUserMutation.mutateAsync(firstUser);
          await signIn(firstUser.email, firstUser.password);
          navigate("/templates");
        }}
      />
    </>
  );
};
