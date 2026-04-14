import type { FC } from "react";
import { Checkbox } from "#/components/Checkbox/Checkbox";

interface RoleOptionProps {
	value: string;
	name: string;
	description: string;
	isChecked: boolean;
	onChange: (roleName: string) => void;
}

export const RoleOption: FC<RoleOptionProps> = ({
	value,
	name,
	description,
	isChecked,
	onChange,
}) => {
	return (
		<label htmlFor={value} className="cursor-pointer">
			<div className="flex items-start gap-4">
				<Checkbox
					id={value}
					checked={isChecked}
					onCheckedChange={() => {
						onChange(value);
					}}
				/>
				<div className="flex flex-col">
					<strong className="text-sm">{name}</strong>
					<span className="text-xs text-content-secondary">{description}</span>
				</div>
			</div>
		</label>
	);
};
