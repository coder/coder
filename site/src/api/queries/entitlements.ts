import { QueryClient } from "@tanstack/react-query";
import * as API from "api/api";
import { Entitlements } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

const ENTITLEMENTS_QUERY_KEY = ["entitlements"];

export const entitlements = () => {
  return {
    queryKey: ENTITLEMENTS_QUERY_KEY,
    queryFn: async () =>
      getMetadataAsJSON<Entitlements>("entitlements") ?? API.getEntitlements(),
  };
};

export const refreshEntitlements = (queryClient: QueryClient) => {
  return {
    mutationFn: API.refreshEntitlements,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ENTITLEMENTS_QUERY_KEY,
      });
    },
  };
};
