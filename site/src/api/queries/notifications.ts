import { API } from "api/api";
import type {
	NotificationPreference,
	NotificationTemplate,
	UpdateNotificationTemplateMethod,
	UpdateUserNotificationPreferences,
} from "api/typesGenerated";
import type { QueryClient, UseMutationOptions } from "react-query";

export const userNotificationPreferencesKey = (userId: string) => [
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
						}) satisfies NotificationPreference,
				),
			);
		},
	} satisfies UseMutationOptions<
		NotificationPreference[],
		unknown,
		UpdateUserNotificationPreferences
	>;
};

export const systemNotificationTemplatesKey = [
	"notifications",
	"templates",
	"system",
];

export const systemNotificationTemplates = () => {
	return {
		queryKey: systemNotificationTemplatesKey,
		queryFn: () => API.getSystemNotificationTemplates(),
	};
};

export function selectTemplatesByGroup(
	data: NotificationTemplate[],
): Record<string, NotificationTemplate[]> {
	const grouped = data.reduce(
		(acc, tpl) => {
			if (!acc[tpl.group]) {
				acc[tpl.group] = [];
			}
			acc[tpl.group].push(tpl);
			return acc;
		},
		{} as Record<string, NotificationTemplate[]>,
	);

	// Sort templates within each group
	for (const group in grouped) {
		grouped[group].sort((a, b) => a.name.localeCompare(b.name));
	}

	// Sort groups by name
	const sortedGroups = Object.keys(grouped).sort((a, b) => a.localeCompare(b));
	const sortedGrouped: Record<string, NotificationTemplate[]> = {};
	for (const group of sortedGroups) {
		sortedGrouped[group] = grouped[group];
	}

	return sortedGrouped;
}

export const notificationDispatchMethodsKey = [
	"notifications",
	"dispatchMethods",
];

export const notificationDispatchMethods = () => {
	return {
		staleTime: Number.POSITIVE_INFINITY,
		queryKey: notificationDispatchMethodsKey,
		queryFn: () => API.getNotificationDispatchMethods(),
	};
};

export const updateNotificationTemplateMethod = (
	templateId: string,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: (req: UpdateNotificationTemplateMethod) =>
			API.updateNotificationTemplateMethod(templateId, req),
		onMutate: (data) => {
			const prevData = queryClient.getQueryData<NotificationTemplate[]>(
				systemNotificationTemplatesKey,
			);
			if (!prevData) {
				return;
			}
			queryClient.setQueryData(
				systemNotificationTemplatesKey,
				prevData.map((tpl) =>
					tpl.id === templateId
						? {
								...tpl,
								method: data.method,
							}
						: tpl,
				),
			);
		},
	} satisfies UseMutationOptions<
		void,
		unknown,
		UpdateNotificationTemplateMethod
	>;
};

export const disableNotification = (
	userId: string,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: async (templateId: string) => {
			const result = await API.putUserNotificationPreferences(userId, {
				template_disabled_map: {
					[templateId]: true,
				},
			});
			return result;
		},
		onSuccess: (data) => {
			queryClient.setQueryData(userNotificationPreferencesKey(userId), data);
		},
	} satisfies UseMutationOptions<NotificationPreference[], unknown, string>;
};
