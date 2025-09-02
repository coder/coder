import AlertTitle from "@mui/material/AlertTitle";
import type { WorkspaceAgent } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { MemoizedInlineMarkdown } from "components/Markdown/Markdown";
import { useProxy } from "contexts/ProxyContext";
import type { FC } from "react";
import { docs } from "utils/docs";

interface WildcardHostnameWarningProps {
	agent: WorkspaceAgent;
}

export const WildcardHostnameWarning: FC<WildcardHostnameWarningProps> = ({
	agent,
}) => {
	const { proxy } = useProxy();

	if (proxy.proxy?.wildcard_hostname) {
		return null;
	}

	const hasSubdomainApps = agent.apps?.some((app) => app.subdomain);
	if (!hasSubdomainApps) {
		return null;
	}

	return (
		<Alert severity="warning">
			<AlertTitle>Some workspace applications will not work</AlertTitle>
			<AlertDetail>
				<div>
					One or more apps in this workspace have{" "}
					<MemoizedInlineMarkdown>`subdomain = true`</MemoizedInlineMarkdown>,
					which requires a Coder deployment with a Wildcard Access URL
					configured. Please contact your administrator.
				</div>
				<div className="mt-2">
					<Link href={docs("/admin/setup#wildcard-access-url")} target="_blank">
						Learn more about wildcard access URL
					</Link>
				</div>
			</AlertDetail>
		</Alert>
	);
};
