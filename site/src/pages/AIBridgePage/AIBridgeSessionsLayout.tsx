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
					Centralized auditing for LLM usage across your organization.{" "}
					<Link
						href={docs("/ai-coder/ai-governance")}
						className="ml-auto"
						target="_blank"
					>
						More about AI Governance
					</Link>
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeSessionsLayout;
