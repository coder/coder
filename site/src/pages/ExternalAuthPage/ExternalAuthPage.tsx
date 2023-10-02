import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  exchangeExternalAuthDevice,
  getExternalAuthDevice,
  getExternalAuthProvider,
} from "api/api";
import { usePermissions } from "hooks";
import { type FC } from "react";
import { useParams } from "react-router-dom";
import ExternalAuthPageView from "./ExternalAuthPageView";
import { ApiErrorResponse } from "api/errors";
import { isAxiosError } from "axios";

const ExternalAuthPage: FC = () => {
  const { provider } = useParams();
  if (!provider) {
    throw new Error("provider must exist");
  }
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
