import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { SetupPageView } from "./SetupPageView";
import { Navigate, useNavigate } from "react-router-dom";
import { useMutation } from "react-query";
import { createFirstUser } from "api/queries/users";

export const SetupPage: FC = () => {
  const { signIn, isConfiguringTheFirstUser, isSignedIn, isSigningIn } =
    useAuth();
  const createFirstUserMutation = useMutation(createFirstUser());
  const setupIsComplete = !isConfiguringTheFirstUser;
  const navigate = useNavigate();

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
        isLoading={createFirstUserMutation.isLoading || isSigningIn}
        error={createFirstUserMutation.error}
        onSubmit={async (firstUser) => {
          await createFirstUserMutation.mutateAsync(firstUser);
          await signIn(firstUser.email, firstUser.password);
          navigate("/");
        }}
      />
    </>
  );
};
