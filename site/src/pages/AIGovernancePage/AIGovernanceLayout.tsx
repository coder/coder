import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Stack } from "components/Stack/Stack";
import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import { AIGovernanceHelpTooltip } from "./AIGovernanceHelpTooltip";

const Language = {
	title: "AI Governance",
	subtitle: "Manage usage for your organization.",
};

const AIGovernanceLayout: FC<PropsWithChildren> = ({
	children = <Outlet />,
}) => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>{Language.title}</span>
						<AIGovernanceHelpTooltip />
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>{Language.subtitle}</PageHeaderSubtitle>
			</PageHeader>
			{children}
		</Margins>
	);
};

export default AIGovernanceLayout;
