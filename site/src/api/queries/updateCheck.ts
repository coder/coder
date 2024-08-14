import { API } from "api/api";

export const updateCheck = () => {
  return {
    queryKey: ["updateCheck"],
    queryFn: () => API.getUpdateCheck(),
  };
};
