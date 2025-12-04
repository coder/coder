import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Tabs, TabsList, TabsTrigger } from "components/Tabs/Tabs";
import type { FC, PropsWithChildren } from "react";
import { NavLink, Outlet, useLocation } from "react-router";
import { cn } from "utils/cn";
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
			<Tabs value={currentTab} className="mb-6">
				<TabsList>
					<NavLink to="request-logs">
						{({ isActive }) => (
							<TabsTrigger
								value="request-logs"
								className={cn(!isActive && "text-content-secondary")}
							>
								Request Logs
							</TabsTrigger>
						)}
					</NavLink>
					<NavLink to="boundary-logs">
						{({ isActive }) => (
							<TabsTrigger
								value="boundary-logs"
								className={cn(!isActive && "text-content-secondary")}
							>
								Boundary Logs
							</TabsTrigger>
						)}
					</NavLink>
				</TabsList>
			</Tabs>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeLayout;
