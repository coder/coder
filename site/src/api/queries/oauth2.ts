import * as API from "api/api";

export const oauth2AppsKey = ["oauth-apps"];

export const oauth2Apps = () => {
  return {
    queryKey: oauth2AppsKey,
    queryFn: () => API.getOAuth2Apps(),
  };
};

export const oauth2App = (id: string) => {
  return {
    queryKey: [oauth2AppsKey, id],
    queryFn: () => API.getOAuth2App(id),
  };
};

export const oauth2AppSecretsKey = ["oauth-app-secrets"];

export const oauth2AppSecrets = (id: string) => {
  return {
    queryKey: [oauth2AppSecretsKey, id],
    queryFn: () => API.getOAuth2AppSecrets(id),
  };
};
