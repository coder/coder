import type { UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { Experiments } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialExperimentsData = getMetadataAsJSON<Experiments>("experiments");
const experimentsKey = ["experiments"] as const;

export const experiments = (): UseQueryOptions<Experiments> => {
  return {
    ...cachedQuery(initialExperimentsData),
    queryKey: experimentsKey,
    queryFn: () => API.getExperiments(),
  } satisfies UseQueryOptions<Experiments>;
};

export const availableExperiments = () => {
  return {
    queryKey: ["availableExperiments"],
    queryFn: async () => API.getAvailableExperiments(),
  };
};
