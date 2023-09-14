import * as API from "api/api";
import { Experiments } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

export const experiments = () => {
  return {
    queryKey: ["experiments"],
    queryFn: async () =>
      getMetadataAsJSON<Experiments>("experiments") ?? API.getExperiments(),
  };
};
