import { useId } from "react";
import { Label } from "#/components/Label/Label";
import { Switch } from "#/components/Switch/Switch";

type SettingSwitchProps = {
	readonly label: string;
	/** Described to assistive technology as the switch's description. */
	readonly description: React.ReactNode;
	readonly checked: boolean;
	readonly disabled: boolean;
	readonly onCheckedChange: (checked: boolean) => void;
};

/** A labeled switch for one setting, with the description below its label. */
export const SettingSwitch: React.FC<SettingSwitchProps> = ({
	label,
	description,
	checked,
	disabled,
	onCheckedChange,
}) => {
	const switchId = useId();
	const descriptionId = useId();
	return (
		<div className="flex items-start justify-between gap-4">
			<div className="flex flex-col gap-1">
				<Label htmlFor={switchId}>{label}</Label>
				{/* A div, because a description can hold paragraphs. */}
				<div
					id={descriptionId}
					className="flex flex-col gap-1 text-xs text-content-secondary"
				>
					{description}
				</div>
			</div>
			<Switch
				id={switchId}
				aria-describedby={descriptionId}
				checked={checked}
				disabled={disabled}
				onCheckedChange={onCheckedChange}
			/>
		</div>
	);
};
