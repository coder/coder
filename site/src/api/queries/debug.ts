import type { QueryClient, UseMutationOptions } from "react-query";
import * as API from "api/api";
import type { HealthSettings, UpdateHealthSettings } from "api/typesGenerated";

export const HEALTH_QUERY_KEY = ["health"];
export const HEALTH_QUERY_SETTINGS_KEY = ["health", "settings"];

export const health = () => ({
  queryKey: HEALTH_QUERY_KEY,
  queryFn: async () => API.getHealth(),
});

export const refreshHealth = (queryClient: QueryClient) => {
  return {
    mutationFn: async () => {
      await queryClient.cancelQueries(HEALTH_QUERY_KEY);
      const newHealthData = await API.getHealth(true);
      queryClient.setQueryData(HEALTH_QUERY_KEY, newHealthData);
    },
  };
};

export const healthSettings = () => {
  return {
    queryKey: HEALTH_QUERY_SETTINGS_KEY,
    queryFn: API.getHealthSettings,
  };
};

export const updateHealthSettings = (
  queryClient: QueryClient,
): UseMutationOptions<
  HealthSettings,
  unknown,
  UpdateHealthSettings,
  unknown
> => {
  return {
    mutationFn: API.updateHealthSettings,
    onSuccess: async (_, newSettings) => {
      await queryClient.invalidateQueries(HEALTH_QUERY_KEY);
      queryClient.setQueryData(HEALTH_QUERY_SETTINGS_KEY, newSettings);
    },
  };
};
