import AlertTitle from "@mui/material/AlertTitle";
import type { WorkspaceResource } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { useProxy } from "contexts/ProxyContext";
import type { FC } from "react";
import { docs } from "utils/docs";

interface WildcardHostnameWarningProps {
	resources: WorkspaceResource[];
}

export const WildcardHostnameWarning: FC<WildcardHostnameWarningProps> = ({
	resources,
}) => {
	const { proxy } = useProxy();

	if (proxy.proxy?.wildcard_hostname) {
		return null;
	}

	const hasSubdomainCoderApp = resources.some((resource) => {
		return resource.agents?.some((agent) =>
			agent.apps?.some((app) => app.subdomain),
		);
	});

	if (!hasSubdomainCoderApp) {
		return null;
	}

	return (
		<Alert
			severity="warning"
			className="rounded-none border-0 border-l-2 border-l-warning border-b-divider"
		>
			<AlertTitle>Workspace applications will not work</AlertTitle>
			<AlertDetail>
				<div>
					This template contains coder_app resources with{" "}
					<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
						subdomain = true
					</code>
					, but subdomain applications are not configured. Users won't be able
					to access these applications until you configure the{" "}
					<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
						--wildcard-access-url
					</code>{" "}
					flag when starting the Coder server.
				</div>
				<div className="flex items-center gap-2 flex-wrap pt-2">
					<Link href={docs("/admin/setup#wildcard-access-url")} target="_blank">
						<span className="font-semibold">
							Learn more about wildcard access URL
						</span>
					</Link>
				</div>
			</AlertDetail>
		</Alert>
	);
};
