import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import { Link } from "#/components/Link/Link";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { docs } from "#/utils/docs";

const AIBridgeSessionsLayout: FC<PropsWithChildren> = () => {
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
					<Link
						href={docs("/ai-coder/ai-bridge/audit")}
						className="ml-auto"
						target="_blank"
					>
						Learn how to audit AI sessions
					</Link>
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeSessionsLayout;
