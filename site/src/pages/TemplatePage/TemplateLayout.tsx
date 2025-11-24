import { API } from "api/api";
import { checkAuthorization } from "api/queries/authCheck";
import type { AuthorizationRequest } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { TabLink, Tabs, TabsList } from "components/Tabs/Tabs";
import { useAuthenticated } from "hooks";
import {
	type WorkspacePermissions,
	workspacePermissionChecks,
} from "modules/permissions/workspaces";
import {
	createContext,
	type FC,
	type PropsWithChildren,
	Suspense,
	useContext,
} from "react";
import { useQuery } from "react-query";
import { Outlet, useLocation, useNavigate, useParams } from "react-router";
import { TemplatePageHeader } from "./TemplatePageHeader";

const templatePermissions = (
	templateId: string,
): AuthorizationRequest["checks"] => ({
	canUpdateTemplate: {
		object: {
			resource_type: "template",
			resource_id: templateId,
		},
		action: "update",
	},
	canReadInsights: {
		object: {
			resource_type: "template",
			resource_id: templateId,
		},
		action: "view_insights",
	},
});

const fetchTemplate = async (organizationId: string, templateName: string) => {
	const template = await API.getTemplateByName(organizationId, templateName);

	const [activeVersion, permissions] = await Promise.all([
		API.getTemplateVersion(template.active_version_id),
		API.checkAuthorization({
			checks: templatePermissions(template.id),
		}),
	]);

	return {
		template,
		activeVersion,
		permissions,
	};
};

type TemplateLayoutContextValue = Awaited<ReturnType<typeof fetchTemplate>>;

const TemplateLayoutContext = createContext<
	TemplateLayoutContextValue | undefined
>(undefined);

export const useTemplateLayoutContext = (): TemplateLayoutContextValue => {
	const context = useContext(TemplateLayoutContext);
	if (!context) {
		throw new Error(
			"useTemplateLayoutContext only can be used inside of TemplateLayout",
		);
	}
	return context;
};

export const TemplateLayout: FC<PropsWithChildren> = ({
	children = <Outlet />,
}) => {
	const navigate = useNavigate();
	const { user: me } = useAuthenticated();
	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const { data, error, isLoading } = useQuery({
		queryKey: ["template", templateName],
		queryFn: () => fetchTemplate(organizationName, templateName),
	});
	const workspacePermissionsQuery = useQuery({
		...checkAuthorization({
			checks: workspacePermissionChecks(
				data?.template.organization_id ?? "",
				me.id,
			),
		}),
		enabled: !!data,
	});

	const location = useLocation();
	const paths = location.pathname.split("/");
	const templateNamePath = paths.at(-1);
	const activeTab =
		templateNamePath === templateName
			? "summary"
			: (templateNamePath as string);
	// Auditors should also be able to view insights, but do not automatically
	// have permission to update templates. Need both checks.
	const shouldShowInsights =
		data?.permissions?.canUpdateTemplate || data?.permissions?.canReadInsights;

	if (error || workspacePermissionsQuery.error) {
		return (
			<div className="p-4">
				<ErrorAlert error={error} />
			</div>
		);
	}

	if (isLoading || !data || !workspacePermissionsQuery.data) {
		return <Loader />;
	}

	return (
		<div className="pb-12">
			<TemplatePageHeader
				template={data.template}
				activeVersion={data.activeVersion}
				permissions={data.permissions}
				workspacePermissions={
					workspacePermissionsQuery.data as WorkspacePermissions
				}
				onDeleteTemplate={() => {
					navigate("/templates");
				}}
			/>

			<Tabs active={activeTab} className="mb-10 -mt-3">
				<Margins>
					<TabsList>
						<TabLink to="docs" value="docs">
							Docs
						</TabLink>
						{data.permissions.canUpdateTemplate && (
							<TabLink to="files" value="files">
								Source Code
							</TabLink>
						)}
						<TabLink to="resources" value="resources">
							Resources
						</TabLink>
						<TabLink to="versions" value="versions">
							Versions
						</TabLink>
						<TabLink to="embed" value="embed">
							Embed
						</TabLink>
						{shouldShowInsights && (
							<TabLink to="insights" value="insights">
								Insights
							</TabLink>
						)}
						{data.permissions.canUpdateTemplate /* TODO template_rbac */ && (
							<TabLink to="prebuilds" value="prebuilds">
								Prebuilds
							</TabLink>
						)}
					</TabsList>
				</Margins>
			</Tabs>

			<Margins>
				<TemplateLayoutContext.Provider value={data}>
					<Suspense fallback={<Loader />}>{children}</Suspense>
				</TemplateLayoutContext.Provider>
			</Margins>
		</div>
	);
};
