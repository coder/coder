import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { SettingsHeaderDocsLink } from "#/components/SettingsHeader/SettingsHeader";
import { docs } from "#/utils/docs";

const AIBridgeSessionsLayout: FC<PropsWithChildren> = () => {
	return (
		<Margins className="pb-12">
			<PageHeader
				actions={
					<SettingsHeaderDocsLink href={docs("/ai-coder/ai-gateway/audit")} />
				}
			>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>AI Sessions</span>
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Review and audit AI activity, token usage, and prompt history across
					sessions.
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeSessionsLayout;
