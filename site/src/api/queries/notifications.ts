import type {
	Mutation,
	MutationFilters,
	QueryClient,
	UseMutationOptions,
} from "react-query";
import { API } from "#/api/api";
import type {
	NotificationPreference,
	NotificationTemplate,
	UpdateNotificationTemplateMethod,
	UpdateUserNotificationPreferences,
} from "#/api/typesGenerated";

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

/** Selects pending choices separately from confirmed notification preferences. */
export const pendingUserNotificationPreferences = (userId: string) => ({
	filters: {
		mutationKey: userNotificationPreferencesKey(userId),
		exact: true,
		status: "pending",
	} satisfies MutationFilters,
	select: ({ mutationId, state }: Mutation) => {
		const preferences: UpdateUserNotificationPreferences["template_disabled_map"] =
			{};
		const request = state.variables;
		if (
			request &&
			typeof request === "object" &&
			"template_disabled_map" in request
		) {
			const map = request.template_disabled_map;
			if (map && typeof map === "object") {
				for (const [id, disabled] of Object.entries(map)) {
					if (typeof disabled === "boolean") {
						preferences[id] = disabled;
					}
				}
			}
		}
		return { mutationId, preferences };
	},
});

export const updateUserNotificationPreferences = (
	userId: string,
	queryClient: QueryClient,
) => {
	const queryKey = userNotificationPreferencesKey(userId);
	return {
		mutationKey: queryKey,
		// Group and individual toggles can update the same notification.
		scope: { id: `user-notification-preferences-${userId}` },
		mutationFn: (req) => {
			return API.putUserNotificationPreferences(userId, req);
		},
		onSuccess: async (data) => {
			await queryClient.cancelQueries({ queryKey });
			queryClient.setQueryData(queryKey, data);
		},
		onSettled: () => {
			// Reconcile once the queue drains without keeping failed choices pending.
			if (
				queryClient.isMutating({ mutationKey: queryKey, exact: true }) === 1
			) {
				void queryClient.invalidateQueries({ queryKey });
			}
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

export const customNotificationTemplatesKey = [
	"notifications",
	"templates",
	"custom",
];

export const customNotificationTemplates = () => {
	return {
		queryKey: customNotificationTemplatesKey,
		queryFn: () => API.getCustomNotificationTemplates(),
	};
};

export function selectTemplatesByGroup(
	data: NotificationTemplate[],
): Record<string, NotificationTemplate[]> {
	const grouped: Record<string, NotificationTemplate[]> = {};
	for (const template of data) {
		if (!grouped[template.group]) {
			grouped[template.group] = [];
		}
		grouped[template.group].push(template);
	}

	// Sort groups by name, and sort templates within each group
	const sortedGroups = Object.keys(grouped).sort((a, b) => a.localeCompare(b));
	const sortedGrouped: Record<string, NotificationTemplate[]> = {};
	for (const group of sortedGroups) {
		sortedGrouped[group] = grouped[group].sort((a, b) =>
			a.name.localeCompare(b.name),
		);
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
			const keys = [
				systemNotificationTemplatesKey,
				customNotificationTemplatesKey,
			];

			for (const key of keys) {
				const prev = queryClient.getQueryData<NotificationTemplate[]>(key);
				if (!prev) {
					continue;
				}

				queryClient.setQueryData(
					key,
					prev.map((tpl) =>
						tpl.id === templateId ? { ...tpl, method: data.method } : tpl,
					),
				);
			}
		},
	} satisfies UseMutationOptions<
		void,
		unknown,
		UpdateNotificationTemplateMethod
	>;
};
