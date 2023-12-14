import * as API from "api/api";

export const oauth2ProviderAppsKey = ["oauth-provider-apps"];

export const oauth2ProviderApps = () => {
  return {
    queryKey: oauth2ProviderAppsKey,
    queryFn: () => API.getOAuth2ProviderApps(),
  };
};

export const oauth2ProviderApp = (id: string) => {
  return {
    queryKey: [oauth2ProviderAppsKey, id],
    queryFn: () => API.getOAuth2ProviderApp(id),
  };
};

export const oauth2ProviderAppSecretsKey = ["oauth-provider-app-secrets"];

export const oauth2ProviderAppSecrets = (id: string) => {
  return {
    queryKey: [oauth2ProviderAppSecretsKey, id],
    queryFn: () => API.getOAuth2ProviderAppSecrets(id),
  };
};
