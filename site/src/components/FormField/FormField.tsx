import { cn } from "cn";
import { type FC, type HTMLAttributes, type ReactNode, useId } from "react";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	type FormHelpers,
	passwordManagerIgnoreProps,
} from "#/utils/formUtils";

type ControlProps = Pick<
	HTMLAttributes<HTMLElement>,
	"id" | "aria-invalid" | "aria-describedby"
>;

type FormFieldProps = React.ComponentPropsWithRef<"input"> & {
	field: FormHelpers;
	label: ReactNode;
	description?: ReactNode;
	/**
	 * When true, discourages password managers (1Password, LastPass, Bitwarden,
	 * etc.) from offering to autofill or save this field. Has no effect when a
	 * custom `control` is provided.
	 */
	ignorePasswordManagers?: boolean;
	/**
	 * Renders in place of the default `Input` element
	 */
	control?: (props: ControlProps) => ReactNode;
};

export const FormField: FC<FormFieldProps> = ({
	field,
	label,
	description,
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
			{description && (
				<div id={descriptionId} className="text-xs text-content-secondary">
					{description}
				</div>
			)}
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
