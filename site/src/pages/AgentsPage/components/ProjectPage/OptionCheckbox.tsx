import { cn } from "cn";
import { CheckIcon } from "lucide-react";
import { useId } from "react";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { Label } from "#/components/Label/Label";

type OptionCheckboxProps = {
	readonly label: string;
	/** A logo or icon shown before the label. */
	readonly icon?: React.ReactNode;
	readonly checked: boolean;
	/** Truly unavailable, such as the only option left. */
	readonly disabled?: boolean;
	/** True while a change saves. The box stays focusable but ignores input. */
	readonly isSaving?: boolean;
	/** Shows a check mark and plain text instead of an inert checkbox. */
	readonly readOnly?: boolean;
	readonly onCheckedChange: (checked: boolean) => void;
};

/** A checkbox with its label, for lists of options an agent may use. */
export const OptionCheckbox: React.FC<OptionCheckboxProps> = ({
	label,
	icon,
	checked,
	disabled,
	isSaving = false,
	readOnly = false,
	onCheckedChange,
}) => {
	const id = useId();
	if (readOnly) {
		return (
			<div className="flex min-w-0 items-center gap-2 text-sm">
				<span className="flex size-4 shrink-0 items-center justify-center">
					{checked && (
						<CheckIcon aria-hidden className="size-4 text-content-success" />
					)}
				</span>
				<span
					className={cn(
						"truncate",
						checked ? "text-content-primary" : "text-content-secondary",
					)}
				>
					{label}
				</span>
				<span className="sr-only">{checked ? "Allowed" : "Not allowed"}</span>
			</div>
		);
	}
	return (
		<div className="flex min-w-0 items-center gap-2 text-sm text-content-primary">
			<Checkbox
				id={id}
				checked={checked}
				disabled={disabled}
				aria-disabled={isSaving}
				className="aria-disabled:cursor-not-allowed aria-disabled:opacity-50"
				onCheckedChange={(value) => {
					if (!isSaving) {
						onCheckedChange(value === true);
					}
				}}
			/>
			<Label
				htmlFor={id}
				className={cn(
					"flex cursor-pointer items-center gap-2 truncate font-normal",
					isSaving && "cursor-not-allowed opacity-50",
				)}
			>
				{icon}
				<span className="truncate">{label}</span>
			</Label>
		</div>
	);
};
