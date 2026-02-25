import type { WorkspaceGitEvent } from "api/types/workspaceGitEvents";
import { useAuthenticated } from "hooks";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { GitEventsPageView } from "./GitEventsPageView";

// TODO: Replace with real data from usePaginatedQuery once the backend
// API endpoint for AI Bridge git events is implemented.
const MOCK_GIT_EVENTS: WorkspaceGitEvent[] = [];

const GitEventsPage: FC = () => {
	const feats = useFeatureVisibility();
	const { permissions } = useAuthenticated();

	const isEntitled = Boolean(feats.aibridge);
	const hasPermission = permissions.viewAnyAIBridgeInterception;

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Git Events", "AI Bridge")}</title>

			<GitEventsPageView
				isLoading={false}
				isVisible={isEntitled}
				events={MOCK_GIT_EVENTS}
			/>
		</RequirePermission>
	);
};

export default GitEventsPage;
