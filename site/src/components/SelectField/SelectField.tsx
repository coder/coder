import type { FC, ReactNode } from "react";
import { FormField } from "#/components/FormField/FormField";
import {
	Select,
	SelectContent,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";

type SelectFieldProps = {
	field: FormHelpers;
	label: ReactNode;
	onValueChange: (value: string) => void;
	children: ReactNode;
	id?: string;
	description?: ReactNode;
	placeholder?: string;
	required?: boolean;
	disabled?: boolean;
	className?: string;
};

/**
 * A labelled Select wired to Formik through getFormHelpers. The label's
 * `htmlFor` targets the trigger's `id`, which makes the ComboBox name accessible.
 */
export const SelectField: FC<SelectFieldProps> = ({
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
