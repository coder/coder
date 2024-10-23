import { buildInfo } from "api/queries/buildInfo";
// import { regions } from "api/queries/regions";
import { authMethods } from "api/queries/users";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type FC, useEffect, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { getApplicationName } from "utils/appearance";
import { retrieveRedirect } from "utils/redirect";
import { sendDeploymentEvent } from "utils/telemetry";
import { LoginPageView } from "./LoginPageView";

export const LoginPage: FC = () => {
	const location = useLocation();
	const {
		isLoading,
		isSignedIn,
		isConfiguringTheFirstUser,
		signIn,
		isSigningIn,
		signInError,
		user,
	} = useAuthContext();
	const authMethodsQuery = useQuery(authMethods());
	const redirectTo = retrieveRedirect(location.search);
	const applicationName = getApplicationName();
	const navigate = useNavigate();
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	let redirectUrl: URL | null = null;
	try {
		redirectUrl = new URL(redirectTo);
	} catch {
		// Do nothing
	}

	const isApiRouteRedirect = redirectTo.startsWith("/api/v2");
	const isReactRedirect =
		(!redirectUrl ||
			(redirectUrl && redirectUrl.host === window.location.host)) &&
		!isApiRouteRedirect;

	useEffect(() => {
		if (!buildInfoQuery.data || isSignedIn) {
			// isSignedIn already tracks with window.href!
			return;
		}
		// This uses `navigator.sendBeacon`, so navigating away will not prevent it!
		sendDeploymentEvent(buildInfoQuery.data, {
			type: "deployment_login",
			user_id: user?.id,
		});
	}, [isSignedIn, buildInfoQuery.data, user?.id]);

	if (isSignedIn && !isReactRedirect) {
		const sanitizedUrl = new URL(redirectTo, window.location.origin);
		window.location.href = sanitizedUrl.pathname + sanitizedUrl.search;
		return null;
	}
	if (isSignedIn && isReactRedirect) {
		return (
			<Navigate to={redirectUrl ? redirectUrl.pathname : redirectTo} replace />
		);
	}

	if (isConfiguringTheFirstUser) {
		return <Navigate to="/setup" replace />;
	}

	return (
		<>
			<Helmet>
				<title>Sign in to {applicationName}</title>
			</Helmet>
			<LoginPageView
				authMethods={authMethodsQuery.data}
				error={signInError}
				isLoading={isLoading || authMethodsQuery.isLoading}
				buildInfo={buildInfoQuery.data}
				isSigningIn={isSigningIn}
				onSignIn={async ({ email, password }) => {
					await signIn(email, password);
					navigate("/");
				}}
			/>
		</>
	);
};

export default LoginPage;
