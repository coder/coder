import { buildInfo } from "api/queries/buildInfo";
import { regions } from "api/queries/regions";
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
	const regionsQuery = useQuery(regions(metadata.regions));
	const [redirectError, setRedirectError] = useState<Error | null>(null);
	let redirectUrl: URL | null = null;
	try {
		redirectUrl = new URL(redirectTo);
	} catch (err) {
		// Do nothing
	}

	const isApiRoute = redirectTo.startsWith("/api/v2");
	const isLocalRedirect =
		(!redirectUrl ||
			(redirectUrl && redirectUrl.host === window.location.host)) &&
		!isApiRoute;

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

	useEffect(() => {
		if (!isSignedIn || !regionsQuery.data || isLocalRedirect) {
			return;
		}

		const regions = regionsQuery.data.regions;
		const pathUrls = regions
			? regions
					.map((region) => {
						try {
							return new URL(region.path_app_url);
						} catch {
							return null;
						}
					})
					.filter((url) => url !== null)
			: [];
		const wildcardHostnames = regions
			? regions
					.map((region) => region.wildcard_hostname)
					.filter((hostname) => hostname !== "")
					// remove the leading '*' from the hostname
					.map((hostname) => hostname.slice(1))
			: [];

		const allowed =
			pathUrls.some((url) => url.host === window.location.host) ||
			wildcardHostnames.some((wildcard) =>
				window.location.host.endsWith(wildcard),
			) ||
			// api routes need to be manually set with href
			isApiRoute;

		if (allowed) {
			window.location.href = redirectTo;
		} else {
			setRedirectError(new Error("invalid redirect url"));
		}
	}, [isSignedIn, regionsQuery.data, redirectTo, isLocalRedirect, isApiRoute]);

	if (isSignedIn && isLocalRedirect) {
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
				error={redirectError ?? signInError}
				isLoading={
					isLoading || authMethodsQuery.isLoading || regionsQuery.isLoading
				}
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
