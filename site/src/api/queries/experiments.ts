import type { UseQueryOptions } from "react-query";
import { client } from "api/api";
import type { Experiments } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialExperimentsData = getMetadataAsJSON<Experiments>("experiments");
const experimentsKey = ["experiments"] as const;

export const experiments = (): UseQueryOptions<Experiments> => {
  return cachedQuery({
    initialData: initialExperimentsData,
    queryKey: experimentsKey,
    queryFn: () => client.api.getExperiments(),
  });
};

export const availableExperiments = () => {
  return {
    queryKey: ["availableExperiments"],
    queryFn: async () => client.api.getAvailableExperiments(),
  };
};
