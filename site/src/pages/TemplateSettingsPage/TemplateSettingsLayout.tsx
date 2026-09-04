import { createContext, type FC, Suspense, useContext } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
import { checkAuthorization } from "#/api/queries/authCheck";
import { templateByName } from "#/api/queries/templates";
import type { AuthorizationResponse, Template } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbLink,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/Breadcrumb/Breadcrumb";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { pageTitle } from "#/utils/page";
import { Sidebar } from "./Sidebar";

const TemplateSettings = createContext<
	{ template: Template; permissions: AuthorizationResponse } | undefined
>(undefined);

export function useTemplateSettings() {
	const value = useContext(TemplateSettings);
	if (!value) {
		throw new Error("This hook can only be used from a template settings page");
	}

	return value;
}

export const TemplateSettingsLayout: FC = () => {
	const { showOrganizations } = useDashboard();
	const { organization: organizationName = "default", template: templateName } =
		useParams() as { organization?: string; template: string };
	const templateQuery = useQuery(
		templateByName(organizationName, templateName),
	);

	const permissionsQuery = useQuery({
		...checkAuthorization({
			checks: {
				canUpdateTemplate: {
					object: {
						resource_type: "template",
						resource_id: templateQuery.data?.id ?? "",
					},
					action: "update",
				},
			},
		}),
		enabled: templateQuery.isSuccess,
	});

	if (!templateQuery.data || !permissionsQuery.data) {
		return <Loader />;
	}

	const error = templateQuery.isError || permissionsQuery.isError;
	// We override `organization_name` here because we may have fallen back to
	// `"default"` while fetching if the deployment does not have the
	// organizations feature enabled, and so we need to make sure consumers do the
	// same when invalidating queries.
	const template = {
		...templateQuery.data,
		organization_name: organizationName,
	};

	return (
		<>
			<title>
				{pageTitle(template?.display_name ?? templateName, "Template Settings")}
			</title>

			<div>
				<Breadcrumb>
					<BreadcrumbList>
						<BreadcrumbItem>
							<BreadcrumbPage>Template Settings</BreadcrumbPage>
						</BreadcrumbItem>
						{template && (
							<>
								{showOrganizations && (
									<>
										<BreadcrumbSeparator />
										<BreadcrumbItem>
											<BreadcrumbPage className="flex items-center gap-2">
												<Avatar
													size="sm"
													fallback={
														template.organization_display_name ||
														template.organization_name
													}
													src={template.organization_icon}
												/>
												{template.organization_display_name}
											</BreadcrumbPage>
										</BreadcrumbItem>
									</>
								)}
								<BreadcrumbSeparator />
								<BreadcrumbItem>
									<BreadcrumbLink to="..">
										<BreadcrumbPage className="flex items-center gap-2">
											<Avatar
												variant="icon"
												size="sm"
												fallback={template.display_name || template.name}
												src={template.icon}
											/>
											{template.display_name || template.name}
										</BreadcrumbPage>
									</BreadcrumbLink>
								</BreadcrumbItem>
							</>
						)}
					</BreadcrumbList>
				</Breadcrumb>
				<div className="h-px border-none bg-border" />

				<section className="px-4 sm:px-6 lg:px-10 max-w-(--breakpoint-2xl) mx-auto">
					<div className="flex flex-col gap-8 py-6 lg:flex-row lg:gap-28 lg:py-10">
						{error ? (
							<ErrorAlert error={error} />
						) : (
							<TemplateSettings.Provider
								value={{
									template: templateQuery.data,
									permissions: permissionsQuery.data,
								}}
							>
								<Sidebar />
								<div className="grow min-w-0">
									<Suspense fallback={<Loader />}>
										<Outlet />
									</Suspense>
								</div>
							</TemplateSettings.Provider>
						)}
					</div>
				</section>
			</div>
		</>
	);
};
