import * as API from "api/api";
import { type UserQuietHoursScheduleResponse } from "api/typesGenerated";
import { type QueryOptions } from "@tanstack/react-query";

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
