import type { Template } from "#/api/typesGenerated";
import { FormField } from "#/components/FormField/FormField";
import { MultiSelectCombobox } from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import type { FormHelpers } from "#/utils/formUtils";
import { NetworkPolicyHCLEditor } from "./NetworkPolicyHCLEditor";

type NetworkPolicyHCLProps = {
	field: FormHelpers;
	onChange: (value: string) => void;
	onBlur: () => void;
	disabled: boolean;
	readOnly: boolean;
};

/** An accessible editor for the restricted, server-validated module configuration. */
export const NetworkPolicyHCL: React.FC<NetworkPolicyHCLProps> = ({
	field,
	onChange,
	onBlur,
	disabled,
	readOnly,
}) => {
	return (
		<FormField
			field={field}
			label="Policy HCL"
			description="Configure the trusted coder_network_policy module. Saving creates an immutable version; publishing applies it independently of workspace builds."
			control={(props) => (
				<NetworkPolicyHCLEditor
					{...props}
					name={field.name}
					value={String(field.value ?? "")}
					onBlur={onBlur}
					onChange={onChange}
					disabled={disabled}
					readOnly={readOnly}
				/>
			)}
		/>
	);
};

type NetworkPolicyTemplateSelectProps = {
	templates: readonly Template[];
	value: readonly string[];
	onChange: (ids: string[]) => void;
	disabled: boolean;
};

/** Keeps unavailable attachments intact rather than silently removing them on save. */
export const NetworkPolicyTemplateSelect: React.FC<
	NetworkPolicyTemplateSelectProps
> = ({ templates, value, onChange, disabled }) => {
	const options = templates.map((template) => ({
		value: template.id,
		label: template.display_name || template.name,
	}));
	return (
		<fieldset className="space-y-2">
			<legend className="text-sm font-medium">Templates</legend>
			<p className="text-sm text-content-secondary">
				Only templates you administer are available. Template labels determine
				which workspaces the policy selects.
			</p>
			<MultiSelectCombobox
				commandProps={{ label: "Templates" }}
				options={options}
				value={value.map(
					(selected) =>
						options.find((option) => option.value === selected) ?? {
							value: selected,
							label: `Unavailable template (${selected})`,
							fixed: true,
						},
				)}
				onChange={(selected) =>
					onChange(selected.map((option) => option.value))
				}
				disabled={disabled}
				placeholder="Select templates"
				emptyIndicator="No administrable templates found."
			/>
		</fieldset>
	);
};
