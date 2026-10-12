import { useId } from "react";
import { Label } from "#/components/Label/Label";
import { RadioGroupItem } from "#/components/RadioGroup/RadioGroup";

type RadioOptionProps = { value: string; label: string; description?: string };

/**
 * One radio with its label. The description is plain text, not a tooltip
 * button: a focusable button inside a Radix radio group makes Shift+Tab
 * land on the group, which hands focus back to the checked radio.
 */
export const RadioOption: React.FC<RadioOptionProps> = ({
	value,
	label,
	description,
}) => {
	const id = useId();
	const descriptionId = useId();
	return (
		<div className="flex items-start gap-2">
			<RadioGroupItem
				id={id}
				value={value}
				className="mt-0.5"
				aria-describedby={description ? descriptionId : undefined}
			/>
			<div className="flex flex-col gap-0.5">
				<Label htmlFor={id} className="font-normal">
					{label}
				</Label>
				{description && (
					<span id={descriptionId} className="text-xs text-content-secondary">
						{description}
					</span>
				)}
			</div>
		</div>
	);
};
