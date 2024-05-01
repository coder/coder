import type { QueryClient, UseQueryOptions } from "react-query";
import { client } from "api/api";
import type { Entitlements } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialEntitlementsData = getMetadataAsJSON<Entitlements>("entitlements");
const entitlementsQueryKey = ["entitlements"] as const;

export const entitlements = (): UseQueryOptions<Entitlements> => {
  return cachedQuery({
    initialData: initialEntitlementsData,
    queryKey: entitlementsQueryKey,
    queryFn: () => client.api.getEntitlements(),
  });
};

export const refreshEntitlements = (queryClient: QueryClient) => {
  return {
    mutationFn: client.api.refreshEntitlements,
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: entitlementsQueryKey,
      });
    },
  };
};
