import AlertTitle from "@mui/material/AlertTitle";
import type { WorkspaceResource } from "api/typesGenerated";
import { Alert, AlertDetail } from "components/Alert/Alert";
import { Link } from "components/Link/Link";
import { useProxy } from "contexts/ProxyContext";
import { useAuthenticated } from "hooks/useAuthenticated";
import type { FC } from "react";
import { docs } from "utils/docs";

interface WildcardHostnameWarningProps {
	// If resources are provided, show template-focused warning
	resources?: WorkspaceResource[];
}

export const WildcardHostnameWarning: FC<WildcardHostnameWarningProps> = ({
	resources,
}) => {
	const { proxy } = useProxy();
	const { permissions } = useAuthenticated();

	const hasResources = Boolean(resources);
	const canEditDeploymentConfig = Boolean(permissions.editDeploymentConfig);

	if (proxy.proxy?.wildcard_hostname) {
		return null;
	}

	if (hasResources) {
		const hasSubdomainCoderApp = resources!.some((resource) => {
			return resource.agents?.some((agent) =>
				agent.apps?.some((app) => app.subdomain),
			);
		});

		if (!hasSubdomainCoderApp) {
			return null;
		}
	}

	return (
		<Alert
			severity="warning"
			prominent
			className={
				hasResources
					? "rounded-none border-0 border-l-2 border-l-warning border-b-divider"
					: undefined
			}
		>
			<AlertTitle>Some workspace applications will not work</AlertTitle>
			<AlertDetail>
				<div>
					{hasResources
						? "This template contains coder_app resources with"
						: "One or more apps in this workspace have"}{" "}
					<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
						subdomain = true
					</code>
					{canEditDeploymentConfig ? (
						<>
							, but subdomain applications are not configured. Users won't be
							able to access these applications until you configure the{" "}
							<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
								--wildcard-access-url
							</code>{" "}
							flag when starting the Coder server.
						</>
					) : (
						", which requires a Coder deployment with a Wildcard Access URL configured. Please contact your administrator."
					)}
				</div>
				<div className="pt-2">
					<Link
						href={docs("/admin/networking/wildcard-access-url")}
						target="_blank"
					>
						<span className="font-semibold">
							Learn more about wildcard access URL
						</span>
					</Link>
				</div>
			</AlertDetail>
		</Alert>
	);
};
