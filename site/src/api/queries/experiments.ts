import * as API from "api/api";
import { Experiments, ExperimentOptions } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

export const experiments = () => {
  return {
    queryKey: ["experiments"],
    queryFn: async () =>
      getMetadataAsJSON<Experiments>("experiments") ?? API.getExperiments(),
  };
};

export const updatedExperiments = (params?: ExperimentOptions) => {
  return {
    queryKey: ["experiments"],
    queryFn: async () => API.getExperiments(params),
  };
};
