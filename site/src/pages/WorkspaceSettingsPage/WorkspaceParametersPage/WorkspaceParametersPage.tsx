import { API } from "api/api";
import { isApiValidationError } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import { richParameters } from "api/queries/templates";
import { workspaceBuildParameters } from "api/queries/workspaceBuilds";
import type {
	TemplateVersionParameter,
	Workspace,
	WorkspaceBuildParameter,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { useMutation, useQuery } from "react-query";
import { useNavigate } from "react-router";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import {
	type WorkspacePermissions,
	workspaceChecks,
} from "../../../modules/workspaces/permissions";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import {
	WorkspaceParametersForm,
	type WorkspaceParametersFormValues,
} from "./WorkspaceParametersForm";

const WorkspaceParametersPage: FC = () => {
	const workspace = useWorkspaceSettings();
	const build = workspace.latest_build;
	const { data: templateVersionParameters } = useQuery(
		richParameters(build.template_version_id),
	);
	const { data: buildParameters } = useQuery(
		workspaceBuildParameters(build.id),
	);
	const navigate = useNavigate();
	const updateParameters = useMutation({
		mutationFn: async (buildParameters: WorkspaceBuildParameter[]) => {
			const currentBuild = workspace.latest_build;

			// If workspace is running, stop it first then start with new parameters
			if (currentBuild.status === "running") {
				const stopBuild = await API.stopWorkspace(workspace.id);
				const awaitedStopBuild = await API.waitForBuild(stopBuild);
				// If the stop is canceled or failed, bail out
				if (awaitedStopBuild?.status === "canceled") {
					throw new Error(
						"Workspace stop was canceled, not proceeding with parameter update.",
					);
				}
				if (awaitedStopBuild?.status === "failed") {
					throw new Error(
						"Workspace failed to stop, not proceeding with parameter update.",
					);
				}
			}

			// Now start the workspace with new parameters
			return API.postWorkspaceBuild(workspace.id, {
				transition: "start",
				rich_parameter_values: buildParameters,
				reason: "dashboard",
			});
		},
		onSuccess: () => {
			navigate(`/${workspace.owner_name}/${workspace.name}`);
		},
	});

	// Permissions
	const checks = workspace ? workspaceChecks(workspace) : {};
	const permissionsQuery = useQuery({
		...checkAuthorization({ checks }),
		enabled: workspace !== undefined,
	});
	const permissions = permissionsQuery.data as WorkspacePermissions | undefined;
	const canChangeVersions = Boolean(permissions?.updateWorkspaceVersion);

	const templatePermissionsQuery = useQuery({
		...checkAuthorization({
			checks: {
				canUpdateTemplate: {
					object: {
						resource_type: "template",
						resource_id: workspace.template_id,
					},
					action: "update",
				},
			},
		}),
		enabled: workspace !== undefined,
	});

	const templatePermissions = templatePermissionsQuery.data as
		| { canUpdateTemplate: boolean }
		| undefined;

	// Check if workspace is in a transitional state
	const isInTransition =
		workspace.latest_build.status === "starting" ||
		workspace.latest_build.status === "stopping" ||
		workspace.latest_build.status === "pending" ||
		workspace.latest_build.status === "canceling";

	return (
		<>
			<title>{pageTitle(workspace.name, "Parameters")}</title>

			<WorkspaceParametersPageView
				workspace={workspace}
				templateVersionParameters={templateVersionParameters}
				buildParameters={buildParameters}
				canChangeVersions={canChangeVersions}
				templatePermissions={templatePermissions}
				isInTransition={isInTransition}
				submitError={updateParameters.error}
				isSubmitting={updateParameters.isPending}
				onSubmit={(values) => {
					if (!templateVersionParameters) {
						return;
					}
					// When updating the parameters, the API does not accept immutable
					// values so we need to filter them
					const onlyMutableValues = templateVersionParameters
						.filter((p) => p.mutable)
						.map((p) => {
							const value = values.rich_parameter_values.find(
								(v) => v.name === p.name,
							);
							if (!value) {
								throw new Error(`Missing value for parameter ${p.name}`);
							}
							return value;
						});
					updateParameters.mutate(onlyMutableValues);
				}}
				onCancel={() => {
					navigate("../..");
				}}
			/>
		</>
	);
};

type WorkspaceParametersPageViewProps = {
	workspace: Workspace;
	canChangeVersions: boolean;
	templatePermissions: { canUpdateTemplate: boolean } | undefined;
	templateVersionParameters?: TemplateVersionParameter[];
	buildParameters?: WorkspaceBuildParameter[];
	isInTransition: boolean;
	submitError: unknown;
	isSubmitting: boolean;
	onSubmit: (formValues: WorkspaceParametersFormValues) => void;
	onCancel: () => void;
};

export const WorkspaceParametersPageView: FC<
	WorkspaceParametersPageViewProps
> = ({
	workspace,
	canChangeVersions,
	templatePermissions,
	templateVersionParameters,
	buildParameters,
	isInTransition,
	submitError,
	onSubmit,
	isSubmitting,
	onCancel,
}) => {
	return (
		<div className="flex flex-col gap-10">
			<header className="flex flex-col items-start gap-2">
				<span className="flex flex-row justify-between w-full items-center gap-2">
					<h1 className="text-3xl m-0">Workspace parameters</h1>
				</span>
			</header>

			{submitError && !isApiValidationError(submitError) ? (
				<ErrorAlert error={submitError} css={{ marginBottom: 48 }} />
			) : null}

			{isInTransition && (
				<Alert severity="info">
					There is currently a workspace build in progress. Please wait for it
					to complete before proceeding.
				</Alert>
			)}

			{templateVersionParameters && buildParameters ? (
				templateVersionParameters.length > 0 ? (
					<WorkspaceParametersForm
						workspace={workspace}
						canChangeVersions={canChangeVersions}
						templatePermissions={templatePermissions}
						isInTransition={isInTransition}
						autofillParams={buildParameters.map((p) => ({
							...p,
							source: "active_build",
						}))}
						templateVersionRichParameters={templateVersionParameters}
						error={submitError}
						isSubmitting={isSubmitting}
						onSubmit={onSubmit}
						onCancel={onCancel}
					/>
				) : (
					<EmptyState
						message="This workspace has no parameters"
						cta={
							<Button asChild>
								<a
									href={docs("/admin/templates/extending-templates/parameters")}
									target="_blank"
									rel="noreferrer"
								>
									<ExternalLinkIcon className="size-icon-xs" />
									Learn more about parameters
								</a>
							</Button>
						}
						css={(theme) => ({
							border: `1px solid ${theme.palette.divider}`,
							borderRadius: 8,
						})}
					/>
				)
			) : (
				<Loader />
			)}
		</div>
	);
};

export default WorkspaceParametersPage;
