import type { WorkspaceResource } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { docs } from "#/utils/docs";

type WildcardHostnameWarningProps = {
	// If resources are provided, show template-focused warning
	resources?: WorkspaceResource[];
};

export const WildcardHostnameWarning: React.FC<
	WildcardHostnameWarningProps
> = ({ resources }) => {
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
			className={
				hasResources
					? "rounded-none border-0 border-l-2 border-l-warning border-b-divider"
					: undefined
			}
			actions={
				<Link
					href={docs("/admin/networking/wildcard-access-url")}
					target="_blank"
					rel="noreferrer"
				>
					Go to wildcard access URL docs
					<span className="sr-only"> (opens in new tab)</span>
				</Link>
			}
		>
			<AlertTitle>Some apps won&apos;t open</AlertTitle>
			<AlertDescription>
				{canEditDeploymentConfig ? (
					<>
						These apps use{" "}
						<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
							subdomain = true
						</code>
						, but this deployment has no wildcard access URL. Set{" "}
						<code className="py-px px-1 bg-surface-tertiary rounded-sm text-content-primary">
							--wildcard-access-url
						</code>{" "}
						on the Coder server so users can open them.
					</>
				) : (
					"These apps need a wildcard access URL, which this deployment doesn't have. Ask your Coder administrator to set one up."
				)}
			</AlertDescription>
		</Alert>
	);
};
