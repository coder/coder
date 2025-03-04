import { buildInfo } from "api/queries/buildInfo";
import { authMethods, createFirstUser } from "api/queries/users";
import { Loader } from "components/Loader/Loader";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery } from "react-query";
import { Navigate, useNavigate } from "react-router-dom";
import { pageTitle } from "utils/page";
import { sendDeploymentEvent } from "utils/telemetry";
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
	const navigate = useNavigate();

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
				authMethods={authMethodsQuery.data}
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
