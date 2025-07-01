import { buildInfo } from "api/queries/buildInfo";
import { authMethods } from "api/queries/users";
import { useAuthContext } from "contexts/auth/AuthProvider";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { type FC, useEffect } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { getApplicationName } from "utils/appearance";
import { retrieveRedirect } from "utils/redirect";
import { sendDeploymentEvent } from "utils/telemetry";
import { LoginPageView } from "./LoginPageView";

const LoginPage: FC = () => {
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
	let redirectError: Error | null = null;
	let redirectUrl: URL | null = null;
	try {
		redirectUrl = new URL(redirectTo);
	} catch {
		// Do nothing
	}

	const isApiRouteRedirect = redirectTo.startsWith("/api/v2");

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

	if (isSignedIn) {
		// The reason we need `window.location.href` for api redirects is that
		// we need the page to reload and make a request to the backend. If we
		// use `<Navigate>`, react would handle the redirect itself and never
		// request the page from the backend.
		if (isApiRouteRedirect) {
			const sanitizedUrl = new URL(redirectTo, window.location.origin);
			window.location.href = sanitizedUrl.pathname + sanitizedUrl.search;
			// Setting the href should immediately request a new page. Show an
			// error state if it doesn't.
			redirectError = new Error("unable to redirect");
		} else {
			return (
				<Navigate
					to={redirectUrl ? redirectUrl.pathname : redirectTo}
					replace
				/>
			);
		}
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
				error={signInError ?? redirectError}
				isLoading={isLoading || authMethodsQuery.isLoading}
				buildInfo={buildInfoQuery.data}
				isSigningIn={isSigningIn}
				onSignIn={async ({ email, password }) => {
					await signIn(email, password);
					navigate("/");
				}}
				redirectTo={redirectTo}
			/>
		</>
	);
};

export default LoginPage;
