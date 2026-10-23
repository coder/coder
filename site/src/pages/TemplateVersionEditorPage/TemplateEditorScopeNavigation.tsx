import { useLocation } from "react-router";
import { LinkTabs, LinkTabsList, TabLink } from "#/components/Tabs/Tabs";

type TemplateEditorScopeNavigationProps = {
	scope: "template" | "workspace";
};

/** Keeps the workspace version and file selection in the URL across scopes. */
export const TemplateEditorScopeNavigation: React.FC<
	TemplateEditorScopeNavigationProps
> = ({ scope }) => {
	const location = useLocation();
	const templateParams = new URLSearchParams(location.search);
	templateParams.set("scope", "template");
	const workspaceParams = new URLSearchParams(location.search);
	workspaceParams.delete("scope");

	return (
		<div className="shrink-0">
			<LinkTabs active={scope}>
				<LinkTabsList
					role="navigation"
					aria-label="Terraform resource scope"
					className="px-4"
				>
					<TabLink to={{ search: templateParams.toString() }} value="template">
						Template resources
					</TabLink>
					<TabLink
						to={{ search: workspaceParams.toString() }}
						value="workspace"
					>
						Workspace resources
					</TabLink>
				</LinkTabsList>
			</LinkTabs>
			<p className="m-0 px-4 py-3 text-sm text-content-secondary border-0 border-b border-solid border-border">
				{scope === "template"
					? "Independent Terraform root, shared by this template. Build saves all files and plans a revision. Publish applies that successful build, not a workspace version."
					: "Independent Terraform root, provisioned for each workspace. Build and publish a template version to update workspace configuration. This does not apply template resources."}
			</p>
		</div>
	);
};
