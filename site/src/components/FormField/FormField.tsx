import {
	type FC,
	type HTMLAttributes,
	type ReactNode,
	useContext,
	useEffect,
	useId,
	useRef,
	useState,
} from "react";
import { HasReachedBottomContext } from "#/components/FormField/HasReachedBottomContext";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { cn } from "#/utils/cn";
import type { FormHelpers } from "#/utils/formUtils";

type ControlProps = Pick<
	HTMLAttributes<HTMLElement>,
	"id" | "aria-invalid" | "aria-describedby"
>;

type FormFieldProps = React.ComponentPropsWithRef<"input"> & {
	field: FormHelpers;
	label: ReactNode;
	description?: ReactNode;
	/**
	 * Renders in place of the default `Input` element
	 */
	control?: (props: ControlProps) => ReactNode;
	/**
	 * When true and the field is `required` with an empty value, the input
	 * flips to `aria-invalid` (destructive red border) once the user scrolls
	 * past it or reaches the bottom of the page. The cue clears as soon as
	 * they type a value.
	 */
	markInvalidWhenScrolledPastEmpty?: boolean;
};

export const FormField: FC<FormFieldProps> = ({
	field,
	label,
	description,
	className,
	control,
	markInvalidWhenScrolledPastEmpty,
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

	// Flip empty required fields to the destructive outline once the user has
	// either scrolled past the field or reached the bottom of the page, so
	// easy-to-miss required fields stand out. The cue clears as soon as the
	// field has a value. Implemented inline (no custom hook) with a sticky
	// IntersectionObserver plus the window-level HasReachedBottomContext.
	const wrapperRef = useRef<HTMLDivElement>(null);
	const [scrolledPast, setScrolledPast] = useState(false);
	const { hasReachedBottom } = useContext(HasReachedBottomContext);

	useEffect(() => {
		if (!markInvalidWhenScrolledPastEmpty) {
			return;
		}
		const el = wrapperRef.current;
		if (!el || typeof IntersectionObserver === "undefined") {
			return;
		}
		let hasBeenSeen = false;
		const observer = new IntersectionObserver(
			(entries) => {
				for (const entry of entries) {
					if (entry.isIntersecting) {
						hasBeenSeen = true;
					} else if (hasBeenSeen && entry.boundingClientRect.top < 0) {
						setScrolledPast(true);
					}
				}
			},
			{ threshold: 0 },
		);
		observer.observe(el);
		return () => observer.disconnect();
	}, [markInvalidWhenScrolledPastEmpty]);

	const isEmpty = field.value == null || field.value === "";
	const showRequiredMiss = Boolean(
		markInvalidWhenScrolledPastEmpty &&
			required &&
			isEmpty &&
			(scrolledPast || hasReachedBottom),
	);
	const isInvalid = Boolean(field.error) || showRequiredMiss;
	const controlProps: ControlProps = {
		id,
		"aria-invalid": isInvalid,
		"aria-describedby": describedBy || undefined,
	};

	return (
		<div ref={wrapperRef} className="flex flex-col gap-2">
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
					{...inputProps}
					{...controlProps}
					className={cn(isInvalid && "border-border-destructive", className)}
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
