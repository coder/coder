import * as API from "api/api";
import { getMetadataAsJSON } from "utils/metadata";
import { type Experiments } from "api/typesGenerated";
import { QueryClient, type QueryOptions } from "react-query";

const initialExperimentsData = getMetadataAsJSON<Experiments>("experiments");
const experimentsKey = ["experiments"] as const;

export const experiments = (queryClient: QueryClient) => {
  return {
    queryKey: experimentsKey,
    queryFn: async () => {
      const cachedData = queryClient.getQueryData(experimentsKey);
      if (cachedData === undefined && initialExperimentsData !== undefined) {
        return initialExperimentsData;
      }

      return API.getExperiments();
    },
  } satisfies QueryOptions<Experiments>;
};
