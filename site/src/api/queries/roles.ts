import { useQuery } from "@tanstack/react-query";
import * as API from "api/api";

export const useRoles = ({ enabled }: { enabled: boolean }) => {
  return useQuery({
    queryKey: ["roles"],
    queryFn: API.getRoles,
    enabled,
  });
};
