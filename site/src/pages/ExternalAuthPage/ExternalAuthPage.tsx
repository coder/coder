import { useQuery, useQueryClient } from "react-query";
import {
  exchangeExternalAuthDevice,
  getExternalAuthDevice,
  getExternalAuthProvider,
} from "api/api";
import { usePermissions } from "hooks";
import { type FC } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import ExternalAuthPageView from "./ExternalAuthPageView";
import { ApiErrorResponse } from "api/errors";
import { isAxiosError } from "axios";
import Button from "@mui/material/Button";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";

const ExternalAuthPage: FC = () => {
  const { provider } = useParams();
  if (!provider) {
    throw new Error("provider must exist");
  }
  const [searchParams] = useSearchParams();
  const permissions = usePermissions();
  const queryClient = useQueryClient();
  const getExternalAuthProviderQuery = useQuery({
    queryKey: ["externalauth", provider],
    queryFn: () => getExternalAuthProvider(provider),
    refetchOnWindowFocus: true,
  });

  const getExternalAuthDeviceQuery = useQuery({
    enabled:
      Boolean(!getExternalAuthProviderQuery.data?.authenticated) &&
      Boolean(getExternalAuthProviderQuery.data?.device),
    queryFn: () => getExternalAuthDevice(provider),
    queryKey: ["externalauth", provider, "device"],
    refetchOnMount: false,
  });
  const exchangeExternalAuthDeviceQuery = useQuery({
    queryFn: () =>
      exchangeExternalAuthDevice(provider, {
        device_code: getExternalAuthDeviceQuery.data?.device_code || "",
      }),
    queryKey: [
      "externalauth",
      provider,
      getExternalAuthDeviceQuery.data?.device_code,
    ],
    enabled: Boolean(getExternalAuthDeviceQuery.data),
    onSuccess: () => {
      // Force a refresh of the Git auth status.
      queryClient.invalidateQueries(["externalauth", provider]).catch((ex) => {
        console.error("invalidate queries", ex);
      });
    },
    retry: true,
    retryDelay: (getExternalAuthDeviceQuery.data?.interval || 5) * 1000,
    refetchOnWindowFocus: (query) =>
      query.state.status === "success" ? false : "always",
  });

  if (
    getExternalAuthProviderQuery.isLoading ||
    !getExternalAuthProviderQuery.data
  ) {
    return null;
  }

  let deviceExchangeError: ApiErrorResponse | undefined;
  if (isAxiosError(exchangeExternalAuthDeviceQuery.failureReason)) {
    deviceExchangeError =
      exchangeExternalAuthDeviceQuery.failureReason.response?.data;
  }

  if (
    !getExternalAuthProviderQuery.data.authenticated &&
    !getExternalAuthProviderQuery.data.device
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
          <Welcome message="Failed to validate oauth access token" />
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
      externalAuth={getExternalAuthProviderQuery.data}
      onReauthenticate={() => {
        queryClient.setQueryData(["externalauth", provider], {
          ...getExternalAuthProviderQuery.data,
          authenticated: false,
        });
      }}
      viewExternalAuthConfig={permissions.viewExternalAuthConfig}
      deviceExchangeError={deviceExchangeError}
      externalAuthDevice={getExternalAuthDeviceQuery.data}
    />
  );
};

export default ExternalAuthPage;
