import { useFormik } from "formik";
import { useId } from "react";
import { useMutation, useQueryClient } from "react-query";
import { updateFoundation } from "#/api/queries/foundations";
import type { Foundation, UpdateFoundationRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { ProvisionerTagsField } from "#/modules/provisioners/ProvisionerTagsField";

import { FoundationDestroyProtection } from "./FoundationDestroyProtection";

type FoundationSettingsProps = { resources: Foundation; canUpdate: boolean };

/** Settings stay local during polling so server refreshes cannot discard edits. */
export const FoundationSettings: React.FC<FoundationSettingsProps> = ({
	resources,
	canUpdate,
}) => {
	const id = useId();
	const client = useQueryClient();
	const mutation = useMutation(
		updateFoundation(client, resources.organization_id, resources.id),
	);
	const form = useFormik<UpdateFoundationRequest>({
		initialValues: {
			name: resources.name,
			provisioner_tags: resources.provisioner_tags,
			prevent_destroy_with_workspaces:
				resources.prevent_destroy_with_workspaces,
		},
		onSubmit: (values) => {
			if (canUpdate && !mutation.isPending)
				mutation.mutate(
					{ ...values, name: values.name.trim() },
					{
						onSuccess: (saved) =>
							form.resetForm({
								values: {
									name: saved.name,
									provisioner_tags: saved.provisioner_tags,
									prevent_destroy_with_workspaces:
										saved.prevent_destroy_with_workspaces,
								},
							}),
					},
				);
		},
	});
	return (
		<form
			className="space-y-3"
			aria-label="Foundation settings"
			onSubmit={form.handleSubmit}
		>
			<h2 className="text-xl font-semibold">Foundation settings</h2>
			<Label htmlFor={`${id}-name`}>Foundation name</Label>
			<Input
				id={`${id}-name`}
				{...form.getFieldProps("name")}
				disabled={!canUpdate || mutation.isPending}
			/>
			<fieldset
				disabled={!canUpdate || mutation.isPending}
				className="space-y-3 border-0 p-0 m-0"
			>
				<legend className="text-sm font-medium mb-2">
					Provisioner routing tags
				</legend>
				<ProvisionerTagsField
					value={form.values.provisioner_tags}
					onChange={(value) =>
						void form.setFieldValue("provisioner_tags", value)
					}
				/>
			</fieldset>
			<p className="text-sm text-content-secondary">
				Routing applies to future Foundation jobs, independently of workspace
				routing. Saving settings does not run Terraform.
			</p>
			<FoundationDestroyProtection
				checked={form.values.prevent_destroy_with_workspaces ?? true}
				disabled={!canUpdate || mutation.isPending}
				onChange={(value) =>
					void form.setFieldValue("prevent_destroy_with_workspaces", value)
				}
			/>
			{Boolean(mutation.error) && <ErrorAlert error={mutation.error} />}
			{mutation.isSuccess && !form.dirty && (
				<p role="status">Foundation settings saved.</p>
			)}
			{!canUpdate && (
				<p>Foundation update permission is required to edit settings.</p>
			)}
			<Button
				disabled={!canUpdate || mutation.isPending || !form.values.name.trim()}
				type="submit"
			>
				Save settings
			</Button>
		</form>
	);
};
