import type { FC } from "react";
import { IconField } from "#/components/IconField/IconField";

interface IconPickerFieldProps {
	id?: string;
	value: string;
	placeholder?: string;
	disabled?: boolean;
	onChange: (value: string) => void;
}

export const IconPickerField: FC<IconPickerFieldProps> = ({
	id,
	value,
	placeholder,
	disabled,
	onChange,
}) => {
	return (
		<IconField
			id={id}
			value={value}
			placeholder={placeholder}
			disabled={disabled}
			label={null}
			onChange={(event) => onChange(event.target.value)}
			onPickEmoji={onChange}
		/>
	);
};
