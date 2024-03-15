import type { QueryClient, UseMutationOptions } from "react-query";
import * as API from "api/api";
import type { ExternalAuth } from "api/typesGenerated";

// Returns all configured external auths for a given user.
export const externalAuths = () => {
  return {
    queryKey: ["external-auth"],
    queryFn: () => API.getUserExternalAuthProviders(),
  };
};

export const externalAuthProvider = (providerId: string) => {
  return {
    queryKey: ["external-auth", providerId],
    queryFn: () => API.getExternalAuthProvider(providerId),
  };
};

export const externalAuthDevice = (providerId: string) => {
  return {
    queryFn: () => API.getExternalAuthDevice(providerId),
    queryKey: ["external-auth", providerId, "device"],
  };
};

export const exchangeExternalAuthDevice = (
  providerId: string,
  deviceCode: string,
  queryClient: QueryClient,
) => {
  return {
    queryFn: () =>
      API.exchangeExternalAuthDevice(providerId, {
        device_code: deviceCode,
      }),
    queryKey: ["external-auth", providerId, "device", deviceCode],
    onSuccess: async () => {
      // Force a refresh of the Git auth status.
      await queryClient.invalidateQueries(["external-auth", providerId]);
    },
  };
};

export const validateExternalAuth = (
  queryClient: QueryClient,
): UseMutationOptions<ExternalAuth, unknown, string> => {
  return {
    mutationFn: API.getExternalAuthProvider,
    onSuccess: (data, providerId) => {
      queryClient.setQueryData(["external-auth", providerId], data);
    },
  };
};

export const unlinkExternalAuths = (queryClient: QueryClient) => {
  return {
    mutationFn: API.unlinkExternalAuthProvider,
    onSuccess: async () => {
      await queryClient.invalidateQueries(["external-auth"]);
    },
  };
};
