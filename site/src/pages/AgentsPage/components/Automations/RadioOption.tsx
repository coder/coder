import { useId } from "react";
import { Label } from "#/components/Label/Label";
import { RadioGroupItem } from "#/components/RadioGroup/RadioGroup";

export const RadioOption: React.FC<{ value: string; label: string }> = ({
	value,
	label,
}) => {
	const id = useId();
	return (
		<div className="flex items-center gap-2">
			<RadioGroupItem id={id} value={value} />
			<Label htmlFor={id} className="font-normal">
				{label}
			</Label>
		</div>
	);
};
