import type { FC, ReactNode } from "react";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";

const Field: FC<{
	label: string;
	id: string;
	error?: boolean;
	helperText?: ReactNode;
	className?: string;
	children: ReactNode;
}> = ({ label, id, error, helperText, className, children }) => (
	<div className={cn("flex flex-col gap-2", className)}>
		<Label htmlFor={id}>{label}</Label>
		{children}
		{helperText && (
			<span
				className={cn(
					"text-xs text-left",
					error ? "text-content-destructive" : "text-content-secondary",
				)}
			>
				{helperText}
			</span>
		)}
	</div>
);

type SelectFieldProps = FormHelpers & {
	label: string;
	className?: string;
	onValueChange: (value: string) => void;
	placeholder?: string;
	children: ReactNode;
	disabled?: boolean;
};

/**
 * A labelled Select wired to Formik through getFormHelpers. The label's
 * `htmlFor` targets the trigger's `id`, which is what gives the combobox its
 * accessible name.
 */
export const SelectField: FC<SelectFieldProps> = ({
	label,
	id,
	error,
	helperText,
	className,
	value,
	onValueChange,
	placeholder,
	children,
	disabled,
}) => (
	<Field
		label={label}
		id={id}
		error={error}
		helperText={helperText}
		className={className}
	>
		<Select
			value={String(value ?? "")}
			onValueChange={onValueChange}
			disabled={disabled}
		>
			<SelectTrigger id={id}>
				<SelectValue placeholder={placeholder} />
			</SelectTrigger>
			<SelectContent>{children}</SelectContent>
		</Select>
	</Field>
);
