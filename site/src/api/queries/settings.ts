import type { QueryClient, QueryOptions } from "react-query";
import { client } from "api/api";
import type {
  UpdateUserQuietHoursScheduleRequest,
  UserQuietHoursScheduleResponse,
} from "api/typesGenerated";

export const userQuietHoursScheduleKey = (userId: string) => [
  "settings",
  userId,
  "quietHours",
];

export const userQuietHoursSchedule = (
  userId: string,
): QueryOptions<UserQuietHoursScheduleResponse> => {
  return {
    queryKey: userQuietHoursScheduleKey(userId),
    queryFn: () => client.api.getUserQuietHoursSchedule(userId),
  };
};

export const updateUserQuietHoursSchedule = (
  userId: string,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (request: UpdateUserQuietHoursScheduleRequest) =>
      client.api.updateUserQuietHoursSchedule(userId, request),
    onSuccess: async () => {
      await queryClient.invalidateQueries(userQuietHoursScheduleKey(userId));
    },
  };
};
