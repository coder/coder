import { useLocation } from "react-router";
import { LinkTabs, LinkTabsList, TabLink } from "#/components/Tabs/Tabs";

type ProvisioningScopeNavigationProps = {
	scope: "shared" | "per-workspace";
};

/** Keeps the workspace version and file selection in the URL across provisioning scopes. */
export const ProvisioningScopeNavigation: React.FC<
	ProvisioningScopeNavigationProps
> = ({ scope }) => {
	const location = useLocation();
	const sharedParams = new URLSearchParams(location.search);
	sharedParams.set("scope", "template");
	const perWorkspaceParams = new URLSearchParams(location.search);
	perWorkspaceParams.delete("scope");

	return (
		<div className="shrink-0">
			<LinkTabs active={scope}>
				<LinkTabsList
					role="navigation"
					aria-label="Provisioning scopes"
					className="px-4"
				>
					<TabLink to={{ search: sharedParams.toString() }} value="shared">
						Shared
					</TabLink>
					<TabLink
						to={{ search: perWorkspaceParams.toString() }}
						value="per-workspace"
					>
						Per-workspace
					</TabLink>
				</LinkTabsList>
			</LinkTabs>
			<p className="m-0 px-4 py-3 text-sm text-content-secondary border-0 border-b border-solid border-border">
				{scope === "shared"
					? "Provision resources once for all workspaces using this template. Shared resources have their own Terraform root, state, and lifecycle. Build saves all files and plans a revision. Publish applies that successful build, not a workspace version."
					: "Define the resources provisioned for each workspace. Per-workspace resources have their own Terraform root, state, and lifecycle. Build and publish a template version to update workspace configuration. This does not apply shared resources."}
			</p>
		</div>
	);
};
