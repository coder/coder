import Chip from "@mui/material/Chip";
import FormHelperText from "@mui/material/FormHelperText";
import { type FC, useId, useMemo } from "react";

export type MultiTextFieldProps = {
	label: string;
	id?: string;
	values: string[];
	onChange: (values: string[]) => void;
};

export const MultiTextField: FC<MultiTextFieldProps> = ({
	label,
	id,
	values,
	onChange,
}) => {
	const baseId = useId();

	const itemIds = useMemo(() => {
		return Array.from(
			{ length: values.length },
			(_, index) => `${baseId}-item-${index}`,
		);
	}, [baseId, values.length]);

	return (
		<div>
			<label
				className="flex flex-wrap min-h-10 px-1.5 py-1.5 gap-2 border border-border border-solid relative rounded-md
				focus-within:border-content-link focus-within:border-2 focus-within:-top-px focus-within:-left-px"
			>
				{values.map((value, index) => (
					<Chip
						key={itemIds[index]}
						className="rounded-md bg-surface-secondary text-content-secondary h-7"
						label={value}
						size="small"
						onDelete={() => {
							onChange(values.filter((oldValue) => oldValue !== value));
						}}
					/>
				))}
				<input
					id={id}
					aria-label={label}
					className="flex-grow text-inherit p-0 border-none bg-transparent focus:outline-none"
					onKeyDown={(event) => {
						if (event.key === ",") {
							event.preventDefault();
							const newValue = event.currentTarget.value;
							onChange([...values, newValue]);
							event.currentTarget.value = "";
							return;
						}

						if (event.key === "Backspace" && event.currentTarget.value === "") {
							event.preventDefault();

							if (values.length === 0) {
								return;
							}

							const lastValue = values[values.length - 1];
							onChange(values.slice(0, -1));
							event.currentTarget.value = lastValue;
						}
					}}
					onBlur={(event) => {
						if (event.currentTarget.value !== "") {
							const newValue = event.currentTarget.value;
							onChange([...values, newValue]);
							event.currentTarget.value = "";
						}
					}}
				/>
			</label>

			<FormHelperText className="text-content-secondary text-xs">
				{'Type "," to separate the values'}
			</FormHelperText>
		</div>
	);
};
