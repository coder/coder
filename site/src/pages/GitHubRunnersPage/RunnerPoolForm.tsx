import { useFormik } from "formik";
import { useState } from "react";
import { useQuery } from "react-query";
import * as Yup from "yup";
import { templatesByOrganization } from "#/api/queries/templates";
import type {
	CreateGitHubRunnerPoolRequest,
	GitHubRunnerPool,
	MinimalUser,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { WorkspaceUserAutocomplete } from "#/components/Autocomplete/WorkspaceUserAutocomplete";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { SelectItem } from "#/components/Select/Select";
import { SelectField } from "#/components/SelectField/SelectField";
import { Spinner } from "#/components/Spinner/Spinner";
import { getFormHelpers, nameValidator } from "#/utils/formUtils";

type RunnerPoolFormProps = {
	organization: string;
	defaultOwner: MinimalUser;
	pool?: GitHubRunnerPool;
	error: unknown;
	isPending: boolean;
	onSubmit: (request: CreateGitHubRunnerPoolRequest) => void;
	onCancel: () => void;
};

/** Keeps initial settings stable while the parent refreshes pool status. */
export const RunnerPoolForm: React.FC<RunnerPoolFormProps> = ({
	organization,
	defaultOwner,
	pool,
	error,
	isPending,
	onSubmit,
	onCancel,
}) => {
	const [owner, setOwner] = useState<MinimalUser | null>(defaultOwner);
	const templatesQuery = useQuery({
		...templatesByOrganization(organization),
		enabled: !pool,
	});
	const form = useFormik<CreateGitHubRunnerPoolRequest>({
		initialValues: {
			name: pool?.name ?? "",
			template_id: pool?.template_id ?? "",
			owner_id: defaultOwner.id,
			github_config_url: pool?.github_config_url ?? "",
			github_token: "",
			runner_group_id: pool?.runner_group_id ?? 1,
			max_runners: pool?.max_runners ?? 2,
			startup_timeout_seconds: pool?.startup_timeout_seconds ?? 600,
			job_timeout_seconds: pool?.job_timeout_seconds ?? 21600,
		},
		validationSchema: Yup.object({
			max_runners: Yup.number()
				.integer()
				.min(1)
				.max(1000)
				.required("Capacity is required."),
			...(!pool && {
				name: nameValidator("Pool name"),
				template_id: Yup.string().required("Choose a runner template."),
				owner_id: Yup.string().required("Choose a workspace owner."),
				github_config_url: Yup.string()
					.matches(
						/^https:\/\/github\.com\/[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)?\/?$/,
						"Enter a GitHub.com organization or repository URL.",
					)
					.required("A GitHub URL is required."),
				github_token: Yup.string().required("A GitHub token is required."),
				runner_group_id: Yup.number()
					.integer()
					.min(1)
					.max(Number.MAX_SAFE_INTEGER)
					.required(),
				startup_timeout_seconds: Yup.number()
					.integer()
					.min(60)
					.max(3600)
					.required(),
				job_timeout_seconds: Yup.number()
					.integer()
					.min(60)
					.max(86400)
					.required(),
			}),
		}),
		onSubmit,
	});
	const field = getFormHelpers(form, error);

	return (
		<form noValidate onSubmit={form.handleSubmit} className="space-y-6">
			{Boolean(error) && <ErrorAlert error={error} />}
			<fieldset
				disabled={isPending}
				className="border-0 p-0 m-0 space-y-5 min-w-0"
			>
				{!pool && (
					<>
						<FormField
							label="Pool name"
							field={field("name")}
							required
							description="Use this name as the runs-on label in your GitHub workflow."
							autoComplete="off"
						/>
						<FormField
							label="GitHub URL"
							field={field("github_config_url")}
							required
							placeholder="https://github.com/organization/repository"
						/>
						<FormField
							label="GitHub token"
							field={field("github_token")}
							required
							type="password"
							ignorePasswordManagers
							autoComplete="new-password"
							description="A PAT with runner administration permissions. It is stored in Coder and is never sent to runner workspaces."
						/>
						{templatesQuery.error && (
							<div className="space-y-2">
								<ErrorAlert error={templatesQuery.error} />
								<Button
									variant="outline"
									onClick={() => void templatesQuery.refetch()}
								>
									Retry templates
								</Button>
							</div>
						)}
						<SelectField
							label="Runner template"
							field={field("template_id")}
							required
							placeholder={
								templatesQuery.isLoading
									? "Loading templates..."
									: "Choose a runner template"
							}
							disabled={!templatesQuery.data}
							onValueChange={(id) => void form.setFieldValue("template_id", id)}
							description="Use a template that starts the GitHub Actions runner with Coder's JIT secret."
						>
							{templatesQuery.data?.map((template) => (
								<SelectItem key={template.id} value={template.id}>
									{template.display_name || template.name}
								</SelectItem>
							))}
						</SelectField>
						{templatesQuery.data?.length === 0 && (
							<p className="text-sm text-content-secondary">
								No templates are available. Import a GitHub Actions runner
								template before creating a pool.
							</p>
						)}
						<WorkspaceUserAutocomplete
							organizationId={organization}
							label="Workspace owner"
							value={owner}
							onChange={(nextOwner) => {
								setOwner(nextOwner);
								void form.setFieldValue("owner_id", nextOwner?.id ?? "");
							}}
						/>
						<p className="text-xs text-content-secondary">
							Choose a dedicated active account without personal secrets. The
							template and workspace permissions of this account apply to every
							runner.
						</p>
						{form.touched.owner_id && form.errors.owner_id && (
							<p className="text-xs text-content-destructive">
								{form.errors.owner_id}
							</p>
						)}
					</>
				)}
				<FormField
					label="Maximum runners"
					type="number"
					min={1}
					max={1000}
					step={1}
					field={field("max_runners")}
					required
					description="Includes workspaces being provisioned or cleaned up."
				/>
				{!pool && (
					<details className="text-sm text-content-secondary">
						<summary className="cursor-pointer">Advanced settings</summary>
						<div className="mt-4 space-y-5">
							<FormField
								label="Runner group ID"
								type="number"
								min={1}
								step={1}
								field={field("runner_group_id")}
								required
							/>
							<FormField
								label="Startup timeout (seconds)"
								type="number"
								min={60}
								max={3600}
								step={1}
								field={field("startup_timeout_seconds")}
								required
							/>
							<FormField
								label="Job timeout (seconds)"
								type="number"
								min={60}
								max={86400}
								step={1}
								field={field("job_timeout_seconds")}
								required
								description="Coder deletes the workspace when this deadline is exceeded, even if the job is still running."
							/>
						</div>
					</details>
				)}
			</fieldset>
			<div className="flex justify-end gap-3">
				<Button variant="outline" onClick={onCancel} disabled={isPending}>
					Cancel
				</Button>
				<Button
					type="submit"
					disabled={isPending || (!pool && !templatesQuery.data?.length)}
				>
					<Spinner loading={isPending} />
					{pool ? "Save capacity" : "Create pool"}
				</Button>
			</div>
		</form>
	);
};
