import * as API from "api/api";
import { QueryClient } from "react-query";

const getUserExternalAuthsKey = () => ["list", "external-auth"];

// listUserExternalAuths returns all configured external auths for a given user.
export const listUserExternalAuths = () => {
  return {
    queryKey: getUserExternalAuthsKey(),
    queryFn: () => API.getUserExternalAuthProviders(),
  };
};

const getUserExternalAuthKey = (providerID: string) => [
  providerID,
  "get",
  "external-auth",
];

export const userExternalAuth = (providerID: string) => {
  return {
    queryKey: getUserExternalAuthKey(providerID),
    queryFn: () => API.getExternalAuthProvider(providerID),
  };
};

export const validateExternalAuth = (_: QueryClient) => {
  return {
    mutationFn: API.getExternalAuthProvider,
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
