import * as API from "api/api";
import { getMetadataAsJSON } from "utils/metadata";
import { type Experiments } from "api/typesGenerated";
import { type UseQueryOptions } from "react-query";

const initialExperimentsData = getMetadataAsJSON<Experiments>("experiments");
const experimentsKey = ["experiments"] as const;

export const experiments = (): UseQueryOptions<Experiments> => {
  return {
    queryKey: experimentsKey,
    initialData: initialExperimentsData,
    queryFn: () => API.getExperiments(),
  } satisfies UseQueryOptions<Experiments>;
};

export const availableExperiments = () => {
  return {
    queryKey: ["availableExperiments"],
    queryFn: async () => API.getAvailableExperiments(),
  };
};
