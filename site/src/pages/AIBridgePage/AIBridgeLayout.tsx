import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import type { FC, PropsWithChildren } from "react";
import { Outlet, useLocation } from "react-router";
import { AIBridgeHelpTooltip } from "./AIBridgeHelpTooltip";

const AIBridgeLayout: FC<PropsWithChildren> = () => {
	const location = useLocation();
	const paths = location.pathname.split("/");
	const activeTab = paths.at(-1) ?? "sessions";

	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>AI Bridge</span>
						<AIBridgeHelpTooltip />
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Centralized auditing for LLM usage across your organization.
				</PageHeaderSubtitle>
			</PageHeader>

			<Tabs active={activeTab} className="mb-10 -mt-3">
				<TabsList>
					<TabLink to="sessions" value="sessions">
						Sessions
					</TabLink>
					<TabLink to="request-logs" value="request-logs">
						Request Logs
					</TabLink>
					<TabLink to="git-events" value="git-events">
						Git Events
					</TabLink>
					<TabLink to="dashboard" value="dashboard">
						Dashboard
					</TabLink>
				</TabsList>
			</Tabs>

			<Outlet />
		</Margins>
	);
};

export default AIBridgeLayout;
