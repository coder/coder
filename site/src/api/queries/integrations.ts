import { GetJFrogXRayScanParams } from "api/api";
import * as API from "api/api";

export const xrayScan = (params: GetJFrogXRayScanParams) => {
  return {
    queryKey: ["xray", params],
    queryFn: () => API.getJFrogXRayScan(params),
  };
};
