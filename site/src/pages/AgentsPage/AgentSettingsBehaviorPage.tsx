// This file exists only as a backward-compat redirect for
// /agents/settings/behavior.
// The old Behavior page was split into General, Compaction,
// System Instructions, Experiments, and Lifecycle pages.
// It now redirects to /agents/settings/general, so keep this as a
// bookmark-preserving alias that should not grow.
import type { FC } from "react";
import { Navigate } from "react-router";

const AgentSettingsBehaviorPage: FC = () => {
	return <Navigate to="/agents/settings/general" replace />;
};

export default AgentSettingsBehaviorPage;
