import Button from "@mui/material/Button";
import { API } from "api/api";
import { isApiValidationError } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery } from "react-query";
import { useNavigate } from "react-router-dom";
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
	const parameters = useQuery({
		queryKey: ["workspace", workspace.id, "parameters"],
		queryFn: () => API.getWorkspaceParameters(workspace),
	});
	const navigate = useNavigate();
	const updateParameters = useMutation({
		mutationFn: (buildParameters: WorkspaceBuildParameter[]) =>
			API.postWorkspaceBuild(workspace.id, {
				transition: "start",
				rich_parameter_values: buildParameters,
			}),
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

	return (
		<>
			<Helmet>
				<title>{pageTitle(workspace.name, "Parameters")}</title>
			</Helmet>

			<WorkspaceParametersPageView
				workspace={workspace}
				canChangeVersions={canChangeVersions}
				data={parameters.data}
				submitError={updateParameters.error}
				isSubmitting={updateParameters.isPending}
				onSubmit={(values) => {
					if (!parameters.data) {
						return;
					}
					// When updating the parameters, the API does not accept immutable
					// values so we need to filter them
					const onlyMultableValues =
						parameters.data.templateVersionRichParameters
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
					updateParameters.mutate(onlyMultableValues);
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
	data: Awaited<ReturnType<typeof API.getWorkspaceParameters>> | undefined;
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
	data,
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

			{data ? (
				data.templateVersionRichParameters.length > 0 ? (
					<WorkspaceParametersForm
						workspace={workspace}
						canChangeVersions={canChangeVersions}
						autofillParams={data.buildParameters.map((p) => ({
							...p,
							source: "active_build",
						}))}
						templateVersionRichParameters={data.templateVersionRichParameters}
						error={submitError}
						isSubmitting={isSubmitting}
						onSubmit={onSubmit}
						onCancel={onCancel}
					/>
				) : (
					<EmptyState
						message="This workspace has no parameters"
						cta={
							<Button
								component="a"
								href={docs("/admin/templates/extending-templates/parameters")}
								startIcon={<ExternalLinkIcon className="size-icon-xs" />}
								variant="contained"
								target="_blank"
								rel="noreferrer"
							>
								Learn more about parameters
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
