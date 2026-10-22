import { useFormik } from "formik";
import type {
	CreateNetworkPolicyRequest,
	Template,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { FormField } from "#/components/FormField/FormField";
import { getFormHelpers } from "#/utils/formUtils";
import {
	NetworkPolicyHCL,
	NetworkPolicyTemplateSelect,
} from "./NetworkPolicyFields";

type CreateNetworkPolicyFormProps = {
	templates: readonly Template[];
	isPending: boolean;
	error: unknown;
	onSubmit: (request: CreateNetworkPolicyRequest) => void;
};

/** Collects an initial immutable revision without publishing infrastructure. */
export const CreateNetworkPolicyForm: React.FC<
	CreateNetworkPolicyFormProps
> = ({ templates, isPending, error, onSubmit }) => {
	const form = useFormik<CreateNetworkPolicyRequest>({
		initialValues: {
			name: "",
			description: "",
			hcl: `module "coder_network_policy" {
  source = "./module"
}
`,
			template_ids: [],
		},
		onSubmit: (values) => onSubmit(values),
	});
	const getField = getFormHelpers(form, error);
	return (
		<form className="space-y-6" onSubmit={form.handleSubmit}>
			{Boolean(error) && <ErrorAlert error={error} />}
			<fieldset disabled={isPending} className="space-y-6">
				<FormField field={getField("name")} label="Name" required />
				<FormField field={getField("description")} label="Description" />
				<NetworkPolicyTemplateSelect
					templates={templates}
					value={form.values.template_ids}
					onChange={(ids) => void form.setFieldValue("template_ids", ids)}
					disabled={isPending}
				/>
				<NetworkPolicyHCL
					readOnly={false}
					field={getField("hcl")}
					disabled={isPending}
					onBlur={() => void form.setFieldTouched("hcl", true)}
					onChange={(hcl) => void form.setFieldValue("hcl", hcl)}
				/>
				<Button
					type="submit"
					disabled={
						isPending || !form.values.name.trim() || !form.values.hcl.trim()
					}
				>
					Create policy
				</Button>
			</fieldset>
		</form>
	);
};
