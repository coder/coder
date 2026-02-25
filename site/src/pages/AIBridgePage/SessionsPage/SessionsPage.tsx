import type { WorkspaceGitEventSession } from "api/types/workspaceGitEvents";
import { useAuthenticated } from "hooks";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { pageTitle } from "utils/page";
import { SessionsPageView } from "./SessionsPageView";

// TODO: Replace with real data from usePaginatedQuery once the backend
// API endpoint for AI Bridge sessions is implemented.
const MOCK_SESSIONS: WorkspaceGitEventSession[] = [];

const SessionsPage: FC = () => {
	const feats = useFeatureVisibility();
	const { permissions } = useAuthenticated();

	const isEntitled = Boolean(feats.aibridge);
	const hasPermission = permissions.viewAnyAIBridgeInterception;

	return (
		<RequirePermission isFeatureVisible={hasPermission}>
			<title>{pageTitle("Sessions", "AI Bridge")}</title>

			<SessionsPageView
				isLoading={false}
				isVisible={isEntitled}
				sessions={MOCK_SESSIONS}
			/>
		</RequirePermission>
	);
};

export default SessionsPage;
