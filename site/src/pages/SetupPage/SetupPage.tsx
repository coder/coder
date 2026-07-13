import { type FC, useEffect, useRef } from "react";
import { useMutation, useQuery } from "react-query";
import { Navigate } from "react-router";
import { buildInfo } from "#/api/queries/buildInfo";
import { authMethods, createFirstUser } from "#/api/queries/users";
import { Loader } from "#/components/Loader/Loader";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { pageTitle } from "#/utils/page";
import { sendDeploymentEvent } from "#/utils/telemetry";
import { SetupPageView } from "./SetupPageView";

export const SetupPage: FC = () => {
	const {
		isLoading,
		signIn,
		isConfiguringTheFirstUser,
		isSignedIn,
		isSigningIn,
	} = useAuthContext();
	const authMethodsQuery = useQuery(authMethods());
	const createFirstUserMutation = useMutation(createFirstUser());
	const setupIsComplete = !isConfiguringTheFirstUser;
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const coderAssistantEnabled =
		metadata.experiments.value?.includes("coder-assistant") ?? false;
	const setupRequired = useRef(false);

	useEffect(() => {
		if (!buildInfoQuery.data) {
			return;
		}
		sendDeploymentEvent(buildInfoQuery.data, {
			type: "deployment_setup",
		});
	}, [buildInfoQuery.data]);

	if (isLoading || authMethodsQuery.isLoading) {
		return <Loader fullscreen />;
	}

	// If the user is logged in, navigate to the app
	if (isSignedIn) {
		// If the Coder Assistant was enabled during setup and the intro hasn't
		// been seen, show the intro page first. Checked via localStorage
		// rather than the setupRequired ref because the component can
		// remount after sign-in, which resets the ref.
		const agentIntroPending = (() => {
			try {
				return (
					localStorage.getItem("coder_agent_enabled") === "true" &&
					localStorage.getItem("coder_agent_intro_completed") !== "true"
				);
			} catch {
				return false;
			}
		})();
		if (agentIntroPending) {
			return <Navigate to="/setup/agent" replace />;
		}
		return setupRequired.current ? (
			<Navigate to="/templates" replace />
		) : (
			<Navigate to="/" state={{ isRedirect: true }} replace />
		);
	}

	// If we've already completed setup, navigate to the login page
	if (setupIsComplete) {
		return <Navigate to="/login" state={{ isRedirect: true }} replace />;
	}

	setupRequired.current = true;

	return (
		<>
			<title>{pageTitle("Set up your account")}</title>
			<SetupPageView
				authMethods={authMethodsQuery.data}
				isLoading={isSigningIn || createFirstUserMutation.isPending}
				error={createFirstUserMutation.error}
				showAssistantToggle={coderAssistantEnabled}
				onSubmit={async (firstUser) => {
					await createFirstUserMutation.mutateAsync(firstUser);
					await signIn(firstUser.email, firstUser.password);
				}}
			/>
		</>
	);
};
