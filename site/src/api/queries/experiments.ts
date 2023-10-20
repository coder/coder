import * as API from "api/api";
import { getMetadataAsJSON } from "utils/metadata";
import { type Experiments } from "api/typesGenerated";
import { QueryClient, type UseQueryOptions } from "react-query";

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
  } satisfies UseQueryOptions<Experiments>;
};

export const availableExperiments = () => {
  return {
    queryKey: ["availableExperiments"],
    queryFn: async () => API.getAvailableExperiments(),
  };
};
