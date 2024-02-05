import type { QueryClient } from "react-query";
import * as API from "api/api";
import type * as TypesGen from "api/typesGenerated";

const appsKey = ["oauth2-provider", "apps"];
const appKey = (id: string) => appsKey.concat(id);
const appSecretsKey = (id: string) => appKey(id).concat("secrets");

export const getApps = () => {
  return {
    queryKey: appsKey,
    queryFn: () => API.getOAuth2ProviderApps(),
  };
};

export const getApp = (id: string) => {
  return {
    queryKey: appKey(id),
    queryFn: () => API.getOAuth2ProviderApp(id),
  };
};

export const postApp = (queryClient: QueryClient) => {
  return {
    mutationFn: API.postOAuth2ProviderApp,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: appsKey,
      });
    },
  };
};

export const putApp = (queryClient: QueryClient) => {
  return {
    mutationFn: ({
      id,
      req,
    }: {
      id: string;
      req: TypesGen.PutOAuth2ProviderAppRequest;
    }) => API.putOAuth2ProviderApp(id, req),
    onSuccess: async (app: TypesGen.OAuth2ProviderApp) => {
      await queryClient.invalidateQueries({
        queryKey: appKey(app.id),
      });
    },
  };
};

export const deleteApp = (queryClient: QueryClient) => {
  return {
    mutationFn: API.deleteOAuth2ProviderApp,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: appsKey,
      });
    },
  };
};

export const getAppSecrets = (id: string) => {
  return {
    queryKey: appSecretsKey(id),
    queryFn: () => API.getOAuth2ProviderAppSecrets(id),
  };
};

export const postAppSecret = (queryClient: QueryClient) => {
  return {
    mutationFn: API.postOAuth2ProviderAppSecret,
    onSuccess: async (
      _: TypesGen.OAuth2ProviderAppSecretFull,
      appId: string,
    ) => {
      await queryClient.invalidateQueries({
        queryKey: appSecretsKey(appId),
      });
    },
  };
};

export const deleteAppSecret = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ appId, secretId }: { appId: string; secretId: string }) =>
      API.deleteOAuth2ProviderAppSecret(appId, secretId),
    onSuccess: async (_: void, { appId }: { appId: string }) => {
      await queryClient.invalidateQueries({
        queryKey: appSecretsKey(appId),
      });
    },
  };
};
