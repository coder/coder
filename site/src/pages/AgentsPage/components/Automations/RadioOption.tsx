import { useId } from "react";
import { InfoTooltip } from "#/components/InfoTooltip/InfoTooltip";
import { Label } from "#/components/Label/Label";
import { RadioGroupItem } from "#/components/RadioGroup/RadioGroup";

type RadioOptionProps = { value: string; label: string; tooltip?: string };

export const RadioOption: React.FC<RadioOptionProps> = ({
	value,
	label,
	tooltip,
}) => {
	const id = useId();
	return (
		<div className="flex items-center gap-2">
			<RadioGroupItem id={id} value={value} />
			<Label htmlFor={id} className="font-normal">
				{label}
			</Label>
			{tooltip && (
				<InfoTooltip size="small" ariaLabel={`About ${label}`}>
					{tooltip}
				</InfoTooltip>
			)}
		</div>
	);
};
