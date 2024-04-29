import type { QueryClient, UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { Entitlements } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialEntitlementsData = getMetadataAsJSON<Entitlements>("entitlements");
const entitlementsQueryKey = ["entitlements"] as const;

export const entitlements = (): UseQueryOptions<Entitlements> => {
  return cachedQuery({
    initialData: initialEntitlementsData,
    queryKey: entitlementsQueryKey,
    queryFn: () => API.getEntitlements(),
  });
};

export const refreshEntitlements = (queryClient: QueryClient) => {
  return {
    mutationFn: API.refreshEntitlements,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: entitlementsQueryKey,
      });
    },
  };
};
