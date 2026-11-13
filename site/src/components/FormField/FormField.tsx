import { cn } from "cn";
import { useId } from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	type FormHelpers,
	passwordManagerIgnoreProps,
} from "#/utils/formUtils";

type ControlProps = Pick<
	React.ComponentProps<"input">,
	"id" | "aria-invalid" | "aria-describedby"
>;

type FormFieldProps = React.ComponentProps<"input"> & {
	field: FormHelpers;
	label: React.ReactNode;
	description?: React.ReactNode;
	descriptionPosition?: "above" | "below";
	/** Discourages password manager autofill. Ignored with a custom `control`. */
	ignorePasswordManagers?: boolean;
	/**
	 * Renders in place of the default `Input` element
	 */
	control?: (props: ControlProps) => React.ReactNode;
};

export const FormField: React.FC<FormFieldProps> = ({
	field,
	label,
	description,
	descriptionPosition = "above",
	className,
	control,
	ignorePasswordManagers,
	...inputProps
}) => {
	const generatedId = useId();
	const id = inputProps.id ?? generatedId;
	const errorId = `${id}-error`;
	const helperId = `${id}-helper`;
	const descriptionId = `${id}-description`;
	const describedBy = [
		description ? descriptionId : null,
		field.error ? errorId : field.helperText ? helperId : null,
	]
		.filter(Boolean)
		.join(" ");
	const required = inputProps.required ?? false;
	const passwordManagerProps = ignorePasswordManagers
		? passwordManagerIgnoreProps
		: {};
	const controlProps: ControlProps = {
		id,
		"aria-invalid": field.error,
		"aria-describedby": describedBy || undefined,
	};
	const descriptionNode = description && (
		<div id={descriptionId} className="text-xs text-content-secondary">
			{description}
		</div>
	);

	return (
		<div className="flex flex-col gap-2">
			<Label htmlFor={id}>
				{label}
				{required && (
					<>
						{" "}
						<span className="text-xs font-bold text-content-destructive">
							*
						</span>
					</>
				)}
			</Label>
			{descriptionPosition === "above" && descriptionNode}
			{control ? (
				control(controlProps)
			) : (
				<Input
					name={field.name}
					value={field.value}
					onChange={field.onChange}
					onBlur={field.onBlur}
					{...passwordManagerProps}
					{...inputProps}
					{...controlProps}
					className={cn(field.error && "border-border-destructive", className)}
				/>
			)}
			{descriptionPosition === "below" && descriptionNode}
			{field.error ? (
				<span id={errorId} className="text-xs text-content-destructive">
					{field.helperText}
				</span>
			) : (
				field.helperText && (
					<span id={helperId} className="text-xs text-content-secondary">
						{field.helperText}
					</span>
				)
			)}
		</div>
	);
};
