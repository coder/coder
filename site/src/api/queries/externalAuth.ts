import type { QueryClient, UseMutationOptions } from "react-query";
import { client } from "api/api";
import type { ExternalAuth } from "api/typesGenerated";

// Returns all configured external auths for a given user.
export const externalAuths = () => {
  return {
    queryKey: ["external-auth"],
    queryFn: () => client.api.getUserExternalAuthProviders(),
  };
};

export const externalAuthProvider = (providerId: string) => {
  return {
    queryKey: ["external-auth", providerId],
    queryFn: () => client.api.getExternalAuthProvider(providerId),
  };
};

export const externalAuthDevice = (providerId: string) => {
  return {
    queryFn: () => client.api.getExternalAuthDevice(providerId),
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
      client.api.exchangeExternalAuthDevice(providerId, {
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
    mutationFn: client.api.getExternalAuthProvider,
    onSuccess: (data, providerId) => {
      queryClient.setQueryData(["external-auth", providerId], data);
    },
  };
};

export const unlinkExternalAuths = (queryClient: QueryClient) => {
  return {
    mutationFn: client.api.unlinkExternalAuthProvider,
    onSuccess: async () => {
      await queryClient.invalidateQueries(["external-auth"]);
    },
  };
};
