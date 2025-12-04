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
	const currentTab = location.pathname.includes("boundary-logs")
		? "boundary-logs"
		: "request-logs";

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
					Manage usage for your organization.
				</PageHeaderSubtitle>
			</PageHeader>
			<Tabs active={currentTab} className="mb-6">
				<TabsList>
					<TabLink to="request-logs" value="request-logs">
						Request Logs
					</TabLink>
					<TabLink to="boundary-logs" value="boundary-logs">
						Boundary Logs
					</TabLink>
				</TabsList>
			</Tabs>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeLayout;
