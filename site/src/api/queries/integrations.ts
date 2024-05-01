import type { GetJFrogXRayScanParams } from "api/api";
import { client } from "api/api";

export const xrayScan = (params: GetJFrogXRayScanParams) => {
  return {
    queryKey: ["xray", params],
    queryFn: () => client.api.getJFrogXRayScan(params),
  };
};
