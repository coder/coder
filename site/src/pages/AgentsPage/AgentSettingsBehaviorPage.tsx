import type { FC } from "react";
import { Navigate } from "react-router";

const AgentSettingsBehaviorPage: FC = () => {
	return <Navigate to="/agents/settings/general" replace />;
};

export default AgentSettingsBehaviorPage;
