import type { QueryClient, UseMutationOptions } from "react-query";
import { API } from "api/api";
import type {
  NotificationPreference,
  UpdateUserNotificationPreferences,
} from "api/typesGenerated";

const userNotificationPreferencesKey = (userId: string) => [
  "users",
  userId,
  "notifications",
  "preferences",
];

export const userNotificationPreferences = (userId: string) => {
  return {
    queryKey: userNotificationPreferencesKey(userId),
    queryFn: () => API.getUserNotificationPreferences(userId),
  };
};

export const updateUserNotificationPreferences = (
  userId: string,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (req) => {
      return API.putUserNotificationPreferences(userId, req);
    },
    onMutate: (data) => {
      queryClient.setQueryData(
        userNotificationPreferencesKey(userId),
        Object.entries(data.template_disabled_map).map(
          ([id, disabled]) =>
            ({
              id,
              disabled,
              updated_at: new Date().toISOString(),
            }) as NotificationPreference,
        ),
      );
    },
  } satisfies UseMutationOptions<
    NotificationPreference[],
    unknown,
    UpdateUserNotificationPreferences
  >;
};

export const systemNotificationTemplatesByGroup = () => {
  return {
    queryKey: ["notifications", "templates", "system"],
    queryFn: () => API.getSystemNotificationTemplates(),
  };
};
