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

const AIBridgeProvidersLayout: FC<PropsWithChildren> = () => {
	return (
		<Margins className="pb-12">
			<PageHeader>
				<PageHeaderTitle>
					<div className="flex items-center gap-2">
						<span>AI Providers</span>
					</div>
				</PageHeaderTitle>
				<PageHeaderSubtitle>
					Configure the LLM providers that AI Bridge routes requests through.{" "}
					<Link
						href={docs("/ai-coder/ai-bridge")}
						className="ml-auto"
						target="_blank"
					>
						Learn how to manage AI providers
					</Link>
				</PageHeaderSubtitle>
			</PageHeader>
			<Outlet />
		</Margins>
	);
};

export default AIBridgeProvidersLayout;
