import Button from "@mui/material/Button";
import type { ApiErrorResponse } from "api/errors";
import {
	exchangeExternalAuthDevice,
	externalAuthDevice,
	externalAuthProvider,
} from "api/queries/externalAuth";
import { isAxiosError } from "axios";
import {
	isExchangeErrorRetryable,
	newRetryDelay,
} from "components/GitDeviceAuth/GitDeviceAuth";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import type { FC } from "react";
import { useMemo } from "react";
import { useQuery, useQueryClient } from "react-query";
import { useParams, useSearchParams } from "react-router-dom";
import ExternalAuthPageView from "./ExternalAuthPageView";

const ExternalAuthPage: FC = () => {
	const { provider } = useParams() as { provider: string };
	const [searchParams] = useSearchParams();
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const externalAuthProviderOpts = externalAuthProvider(provider);
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
			queryClient,
		),
		enabled: Boolean(externalAuthDeviceQuery.data),
		retry: isExchangeErrorRetryable,
		retryDelay,
		// We don't want to refetch the query outside of the standard retry
		// logic, because the device auth flow is very strict about rate limits.
		refetchOnWindowFocus: false,
	});

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

	if (
		!externalAuthProviderQuery.data.authenticated &&
		!externalAuthProviderQuery.data.device
	) {
		const redirectedParam = searchParams?.get("redirected");
		if (redirectedParam && redirectedParam.toLowerCase() === "true") {
			// The auth flow redirected the user here. If we redirect back to the
			// callback, that resets the flow and we'll end up in an infinite loop.
			// So instead, show an error, as the user expects to be authenticated at
			// this point.
			// TODO: Unsure what to do about the device auth flow, should we also
			// show an error there?
			return (
				<SignInLayout>
					<Welcome>Failed to validate oauth access token</Welcome>

					<p css={{ textAlign: "center" }}>
						Attempted to validate the user&apos;s oauth access token from the
						authentication flow. This situation may occur as a result of an
						external authentication provider misconfiguration. Verify the
						external authentication validation URL is accurately configured.
					</p>
					<br />
					<Button
						onClick={() => {
							// Redirect to the auth flow again. *crosses fingers*
							window.location.href = `/external-auth/${provider}/callback`;
						}}
					>
						Retry
					</Button>
				</SignInLayout>
			);
		}
		window.location.href = `/external-auth/${provider}/callback`;
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
