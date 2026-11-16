import { Outlet } from "react-router";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { SettingsHeaderDocsLink } from "#/components/SettingsHeader/SettingsHeader";
import { docs } from "#/utils/docs";

const AIBridgeSessionsLayout: React.FC<React.PropsWithChildren> = () => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>AI Sessions</span>
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Review and audit AI activity, token usage, and prompt history across
					sessions.{" "}
					<SettingsHeaderDocsLink
						href={docs("/ai-governance/ai-gateway/audit")}
					/>
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeSessionsLayout;
