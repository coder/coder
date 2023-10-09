import * as API from "api/api";
import {
  type UserQuietHoursScheduleResponse,
  type UpdateUserQuietHoursScheduleRequest,
} from "api/typesGenerated";
import { type QueryClient, type QueryOptions } from "react-query";

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
    queryFn: () => API.getUserQuietHoursSchedule(userId),
  };
};

export const updateUserQuietHoursSchedule = (
  userId: string,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (request: UpdateUserQuietHoursScheduleRequest) =>
      API.updateUserQuietHoursSchedule(userId, request),
    onSuccess: async () => {
      await queryClient.invalidateQueries(userQuietHoursScheduleKey(userId));
    },
  };
};
