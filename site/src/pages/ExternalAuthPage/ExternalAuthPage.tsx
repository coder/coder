import { isAxiosError } from "axios";
import type { FC } from "react";
import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router";
import type { ApiErrorResponse } from "#/api/errors";
import {
	exchangeExternalAuthDevice,
	externalAuthDevice,
	externalAuthProvider,
} from "#/api/queries/externalAuth";
import { Button } from "#/components/Button/Button";
import {
	isExchangeErrorRetryable,
	newRetryDelay,
} from "#/components/GitDeviceAuth/GitDeviceAuth";
import { SignInLayout } from "#/components/SignInLayout/SignInLayout";
import { Welcome } from "#/components/Welcome/Welcome";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import ExternalAuthPageView from "./ExternalAuthPageView";

const ExternalAuthPage: FC = () => {
	const { provider } = useParams() as { provider: string };
	const [searchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const externalAuthProviderOpts = useMemo(
		() => externalAuthProvider(provider),
		[provider],
	);
	const externalAuthProviderQuery = useQuery({
		...externalAuthProviderOpts,
		refetchOnWindowFocus: true,
	});

	const externalAuthDeviceQuery = useQuery({
		...externalAuthDevice(provider),
		enabled:
			Boolean(!externalAuthProviderQuery.data?.authenticated) &&
			Boolean(externalAuthProviderQuery.data?.device),
		refetchOnMount: false,
	});
	const retryDelay = useMemo(
		() => newRetryDelay(externalAuthDeviceQuery.data?.interval),
		[externalAuthDeviceQuery.data],
	);
	const exchangeExternalAuthDeviceQuery = useQuery({
		...exchangeExternalAuthDevice(
			provider,
			externalAuthDeviceQuery.data?.device_code ?? "",
		),
		enabled: Boolean(externalAuthDeviceQuery.data),
		retry: isExchangeErrorRetryable,
		retryDelay,
		// We don't want to refetch the query outside of the standard retry
		// logic, because the device auth flow is very strict about rate limits.
		refetchOnWindowFocus: false,
	});

	// Flip the UI out of polling once the exchange succeeds. Replaces the
	// `onSuccess` that react-query v5 dropped from `useQuery`. `exact` avoids
	// re-POSTing the one-time device code.
	useEffect(() => {
		if (!exchangeExternalAuthDeviceQuery.isSuccess) {
			return;
		}
		queryClient.invalidateQueries({
			queryKey: externalAuthProviderOpts.queryKey,
			exact: true,
		});
	}, [
		exchangeExternalAuthDeviceQuery.isSuccess,
		externalAuthProviderOpts.queryKey,
		queryClient,
	]);

	const redirectedParam = searchParams?.get("redirected");
	const alreadyRedirected =
		redirectedParam !== null && redirectedParam.toLowerCase() === "true";
	const needsReauth =
		!externalAuthProviderQuery.data?.authenticated &&
		!externalAuthProviderQuery.data?.device;

	// Kick the user back into the auth flow. When they come back with
	// redirected=true, showing the flow again would loop forever, so instead
	// the error below renders.
	useEffect(() => {
		if (!needsReauth || alreadyRedirected) {
			return;
		}
		location.href = `/external-auth/${provider}/callback`;
	}, [needsReauth, alreadyRedirected, provider]);

	if (externalAuthProviderQuery.isLoading || !externalAuthProviderQuery.data) {
		return null;
	}

	let deviceExchangeError: ApiErrorResponse | undefined;
	if (isAxiosError(exchangeExternalAuthDeviceQuery.failureReason)) {
		deviceExchangeError =
			exchangeExternalAuthDeviceQuery.failureReason.response?.data;
	} else if (isAxiosError(externalAuthDeviceQuery.failureReason)) {
		deviceExchangeError = externalAuthDeviceQuery.failureReason.response?.data;
	}

	if (needsReauth) {
		if (alreadyRedirected) {
			// The auth flow redirected the user here. If we redirect back to the
			// callback, that resets the flow and we'll end up in an infinite loop.
			// So instead, show an error, as the user expects to be authenticated at
			// this point.
			// TODO: Unsure what to do about the device auth flow, should we also
			// show an error there?
			return (
				<SignInLayout>
					<Welcome>Failed to validate oauth access token</Welcome>

					<p className="text-center">
						Attempted to validate the user&apos;s oauth access token from the
						authentication flow. This situation may occur as a result of an
						external authentication provider misconfiguration. Verify the
						external authentication validation URL is accurately configured.
					</p>
					<br />
					<Button
						variant="outline"
						onClick={() => {
							// Redirect to the auth flow again. *crosses fingers*
							location.href = `/external-auth/${provider}/callback`;
						}}
					>
						Retry
					</Button>
				</SignInLayout>
			);
		}
		// The effect above triggers the redirect; render nothing meanwhile.
		return null;
	}

	return (
		<ExternalAuthPageView
			externalAuth={externalAuthProviderQuery.data}
			onReauthenticate={() => {
				queryClient.setQueryData(externalAuthProviderOpts.queryKey, {
					...externalAuthProviderQuery.data,
					authenticated: false,
				});
			}}
			viewExternalAuthConfig={permissions.viewDeploymentConfig}
			deviceExchangeError={deviceExchangeError}
			externalAuthDevice={externalAuthDeviceQuery.data}
		/>
	);
};

export default ExternalAuthPage;
