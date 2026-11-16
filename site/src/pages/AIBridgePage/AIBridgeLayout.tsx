import { Outlet } from "react-router";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { AIBridgeHelpPopover } from "./AIBridgeHelpPopover";

const AIBridgeLayout: React.FC<React.PropsWithChildren> = () => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle tooltip={<AIBridgeHelpPopover />}>
					AI Gateway Logs
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
