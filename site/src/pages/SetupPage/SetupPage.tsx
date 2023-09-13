import { useMachine } from "@xstate/react";
import { useAuth } from "components/AuthProvider/AuthProvider";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { pageTitle } from "utils/page";
import { setupMachine } from "xServices/setup/setupXService";
import { SetupPageView } from "./SetupPageView";
import { Navigate } from "react-router-dom";

export const SetupPage: FC = () => {
  const [authState, authSend] = useAuth();
  const [setupState, setupSend] = useMachine(setupMachine, {
    actions: {
      onCreateFirstUser: ({ firstUser }) => {
        if (!firstUser) {
          throw new Error("First user was not defined.");
        }
        authSend({
          type: "SIGN_IN",
          email: firstUser.email,
          password: firstUser.password,
        });
      },
    },
  });
  const { error } = setupState.context;

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
        isLoading={setupState.hasTag("loading")}
        error={error}
        onSubmit={(firstUser) => {
          setupSend({ type: "CREATE_FIRST_USER", firstUser });
        }}
      />
    </>
  );
};
