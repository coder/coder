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

export const validateExternalAuth = (_: QueryClient) => {
  return {
    mutationFn: API.getExternalAuthProvider,
    onSuccess: async () => {
      // No invalidation needed.
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
