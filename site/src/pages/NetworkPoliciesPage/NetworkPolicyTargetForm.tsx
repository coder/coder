import { useFormik } from "formik";
import type { CreateNetworkPolicyTargetRequest } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { Textarea } from "#/components/Textarea/Textarea";
import { getFormHelpers } from "#/utils/formUtils";

type NetworkPolicyTargetFormProps = {
	generation: number;
	isPending: boolean;
	error: unknown;
	onSubmit: (
		request: CreateNetworkPolicyTargetRequest,
		onSuccess: () => void,
	) => void;
};

/** Registers explicit infrastructure coordinates without starting a provisioner job. */
export const NetworkPolicyTargetForm: React.FC<
	NetworkPolicyTargetFormProps
> = ({ generation, isPending, error, onSubmit }) => {
	const form = useFormik({
		initialValues: { cluster: "", context: "", namespace: "", tags: "" },
		onSubmit: ({ cluster, context, namespace, tags }) => {
			const entries = tags
				.split("\n")
				.map((line) => line.trim())
				.filter(Boolean)
				.map((line) => {
					const separator = line.indexOf("=");
					return separator > 0
						? [
								line.slice(0, separator).trim(),
								line.slice(separator + 1).trim(),
							]
						: [];
				});
			if (
				entries.some((entry) => entry.length !== 2 || !entry[0] || !entry[1]) ||
				new Set(entries.map((entry) => entry[0])).size !== entries.length
			) {
				form.setFieldError(
					"tags",
					"Enter unique provisioner tags as key=value, one per line.",
				);
				return;
			}
			onSubmit(
				{
					name: `${cluster}/${context}/${namespace}`,
					cluster,
					context,
					namespace,
					provisioner_tags: Object.fromEntries(entries),
					expected_generation: generation,
				},
				() => form.resetForm(),
			);
		},
	});
	const getField = getFormHelpers(form, error);
	return (
		<form className="space-y-4" onSubmit={form.handleSubmit}>
			{Boolean(error) && <ErrorAlert error={error} />}
			<p className="text-sm text-content-secondary">
				Targets are explicit. The provisioner must have credentials for this
				cluster and context and advertise the coder.network_policy=1 capability
				tag. The server adds this required tag automatically. Adding a target
				does not apply resources.
			</p>
			<fieldset disabled={isPending} className="grid gap-4 sm:grid-cols-2">
				<FormField field={getField("cluster")} label="Cluster" required />
				<FormField
					field={getField("context")}
					label="Kubernetes context"
					required
				/>
				<FormField field={getField("namespace")} label="Namespace" required />
				<div className="sm:col-span-2">
					<FormField
						field={getField("tags")}
						label="Provisioner tags"
						description="One key=value pair per line. These select the provisioner, not workspace labels."
						control={(props) => (
							<Textarea
								{...props}
								{...form.getFieldProps("tags")}
								placeholder="region=eu-west-1"
							/>
						)}
					/>
				</div>
				<Button type="submit" disabled={isPending}>
					Add target
				</Button>
			</fieldset>
		</form>
	);
};
