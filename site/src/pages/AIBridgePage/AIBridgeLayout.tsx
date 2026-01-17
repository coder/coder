import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import { AIBridgeHelpTooltip } from "./AIBridgeHelpTooltip";

const AIBridgeLayout: FC<PropsWithChildren> = () => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>AI bridge logs</span>
						<AIBridgeHelpTooltip />
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Centralized auditing for LLM usage across your organization.
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeLayout;
