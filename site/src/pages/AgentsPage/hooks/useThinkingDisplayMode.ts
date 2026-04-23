import { useMutation, useQuery, useQueryClient } from "react-query";
import {
	preferenceSettings,
	updatePreferenceSettings,
} from "#/api/queries/users";
import type { ThinkingDisplayMode } from "#/api/typesGenerated";

/**
 * Reactive hook for the thinking display mode preference. Falls
 * back to "auto" while the preference is loading or unset.
 */
export function useThinkingDisplayMode(): {
	mode: ThinkingDisplayMode;
	setMode: (mode: ThinkingDisplayMode) => void;
	isLoading: boolean;
} {
	const queryClient = useQueryClient();
	const query = useQuery(preferenceSettings());
	const mutation = useMutation(updatePreferenceSettings(queryClient));

	const mode: ThinkingDisplayMode = query.data?.thinking_display_mode || "auto";

	const setMode = (next: ThinkingDisplayMode) => {
		mutation.mutate({
			task_notification_alert_dismissed:
				query.data?.task_notification_alert_dismissed ?? false,
			thinking_display_mode: next,
		});
	};

	return { mode, setMode, isLoading: query.isLoading };
}
