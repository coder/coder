import { cn } from "cn";
import { FormField } from "#/components/FormField/FormField";
import {
	Select,
	SelectContent,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import type { FormHelpers } from "#/utils/formUtils";

type SelectFieldProps = {
	field: FormHelpers;
	label: React.ReactNode;
	onValueChange: (value: string) => void;
	children: React.ReactNode;
	id?: string;
	description?: React.ReactNode;
	placeholder?: string;
	required?: boolean;
	disabled?: boolean;
	className?: string;
};

/**
 * A labelled Select wired to Formik through getFormHelpers. The label's
 * `htmlFor` targets the trigger's `id`, which makes the ComboBox name accessible.
 */
export const SelectField: React.FC<SelectFieldProps> = ({
	field,
	label,
	onValueChange,
	children,
	id,
	description,
	placeholder,
	required,
	disabled,
	className,
}) => (
	<FormField
		field={field}
		label={label}
		description={description}
		id={id}
		required={required}
		control={(controlProps) => (
			<Select
				value={String(field.value ?? "")}
				onValueChange={onValueChange}
				disabled={disabled}
			>
				<SelectTrigger
					{...controlProps}
					aria-required={required}
					className={cn(field.error && "border-border-destructive", className)}
				>
					<SelectValue placeholder={placeholder} />
				</SelectTrigger>
				<SelectContent>{children}</SelectContent>
			</Select>
		)}
	/>
);
