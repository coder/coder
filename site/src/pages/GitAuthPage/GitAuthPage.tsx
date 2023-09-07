import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  exchangeGitAuthDevice,
  getGitAuthDevice,
  getGitAuthProvider,
} from "api/api";
import { usePermissions } from "hooks";
import { FC, useEffect } from "react";
import { useParams } from "react-router-dom";
import GitAuthPageView from "./GitAuthPageView";
import { ApiErrorResponse } from "api/errors";
import { isAxiosError } from "axios";
import { REFRESH_GITAUTH_BROADCAST_CHANNEL } from "utils/gitAuth";

const GitAuthPage: FC = () => {
  const { provider } = useParams();
  if (!provider) {
    throw new Error("provider must exist");
  }
  const permissions = usePermissions();
  const queryClient = useQueryClient();
  const getGitAuthProviderQuery = useQuery({
    queryKey: ["gitauth", provider],
    queryFn: () => getGitAuthProvider(provider),
    refetchOnWindowFocus: true,
  });

  const getGitAuthDeviceQuery = useQuery({
    enabled:
      Boolean(!getGitAuthProviderQuery.data?.authenticated) &&
      Boolean(getGitAuthProviderQuery.data?.device),
    queryFn: () => getGitAuthDevice(provider),
    queryKey: ["gitauth", provider, "device"],
    refetchOnMount: false,
  });
  const exchangeGitAuthDeviceQuery = useQuery({
    queryFn: () =>
      exchangeGitAuthDevice(provider, {
        device_code: getGitAuthDeviceQuery.data?.device_code || "",
      }),
    queryKey: ["gitauth", provider, getGitAuthDeviceQuery.data?.device_code],
    enabled: Boolean(getGitAuthDeviceQuery.data),
    onSuccess: () => {
      // Force a refresh of the Git auth status.
      queryClient.invalidateQueries(["gitauth", provider]).catch((ex) => {
        console.error("invalidate queries", ex);
      });
    },
    retry: true,
    retryDelay: (getGitAuthDeviceQuery.data?.interval || 5) * 1000,
    refetchOnWindowFocus: (query) =>
      query.state.status === "success" ? false : "always",
  });

  useEffect(() => {
    if (!getGitAuthProviderQuery.data?.authenticated) {
      return;
    }
    // This is used to notify the parent window that the Git auth token has been refreshed.
    // It's critical in the create workspace flow!
    const bc = new BroadcastChannel(REFRESH_GITAUTH_BROADCAST_CHANNEL);
    // The message doesn't matter, any message refreshes the page!
    bc.postMessage("noop");
  }, [getGitAuthProviderQuery.data?.authenticated]);

  if (getGitAuthProviderQuery.isLoading || !getGitAuthProviderQuery.data) {
    return null;
  }

  let deviceExchangeError: ApiErrorResponse | undefined;
  if (isAxiosError(exchangeGitAuthDeviceQuery.failureReason)) {
    deviceExchangeError =
      exchangeGitAuthDeviceQuery.failureReason.response?.data;
  }

  if (
    !getGitAuthProviderQuery.data.authenticated &&
    !getGitAuthProviderQuery.data.device
  ) {
    window.location.href = `/gitauth/${provider}/callback`;

    return null;
  }

  return (
    <GitAuthPageView
      gitAuth={getGitAuthProviderQuery.data}
      onReauthenticate={() => {
        queryClient.setQueryData(["gitauth", provider], {
          ...getGitAuthProviderQuery.data,
          authenticated: false,
        });
      }}
      viewGitAuthConfig={permissions.viewGitAuthConfig}
      deviceExchangeError={deviceExchangeError}
      gitAuthDevice={getGitAuthDeviceQuery.data}
    />
  );
};

export default GitAuthPage;
