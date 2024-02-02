import Button from "@mui/material/Button";
import { useQuery, useQueryClient } from "react-query";
import { isAxiosError } from "axios";
import { type FC } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { ApiErrorResponse } from "api/errors";
import {
  externalAuthDevice,
  externalAuthProvider,
  exchangeExternalAuthDevice,
} from "api/queries/externalAuth";
import { usePermissions } from "contexts/auth/usePermissions";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import ExternalAuthPageView from "./ExternalAuthPageView";

const ExternalAuthPage: FC = () => {
  const { provider } = useParams() as { provider: string };
  const [searchParams] = useSearchParams();
  const permissions = usePermissions();
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
  const exchangeExternalAuthDeviceQuery = useQuery({
    ...exchangeExternalAuthDevice(
      provider,
      externalAuthDeviceQuery.data?.device_code ?? "",
      queryClient,
    ),
    enabled: Boolean(externalAuthDeviceQuery.data),
    retry: true,
    retryDelay: (externalAuthDeviceQuery.data?.interval || 5) * 1000,
    refetchOnWindowFocus: (query) =>
      query.state.status === "success" ? false : "always",
  });

  if (externalAuthProviderQuery.isLoading || !externalAuthProviderQuery.data) {
    return null;
  }

  let deviceExchangeError: ApiErrorResponse | undefined;
  if (isAxiosError(exchangeExternalAuthDeviceQuery.failureReason)) {
    deviceExchangeError =
      exchangeExternalAuthDeviceQuery.failureReason.response?.data;
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
      viewExternalAuthConfig={permissions.viewExternalAuthConfig}
      deviceExchangeError={deviceExchangeError}
      externalAuthDevice={externalAuthDeviceQuery.data}
    />
  );
};

export default ExternalAuthPage;
