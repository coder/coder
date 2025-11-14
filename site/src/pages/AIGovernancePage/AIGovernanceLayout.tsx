import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import { AIGovernanceHelpTooltip } from "./AIGovernanceHelpTooltip";

const AIGovernanceLayout: FC<PropsWithChildren> = () => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>AI Governance</span>
						<AIGovernanceHelpTooltip />
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Manage usage for your organization.
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIGovernanceLayout;
