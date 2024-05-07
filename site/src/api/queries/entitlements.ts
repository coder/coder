import type { QueryClient } from "react-query";
import { client } from "api/api";
import type { Entitlements } from "api/typesGenerated";
import type { MetadataState } from "hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

const entitlementsQueryKey = ["entitlements"] as const;

export const entitlements = (metadata: MetadataState<Entitlements>) => {
  return cachedQuery({
    metadata,
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
