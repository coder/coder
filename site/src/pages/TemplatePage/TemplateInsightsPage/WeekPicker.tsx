import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { Button } from "components/Button/Button";
import dayjs from "dayjs";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";
import type { DateRangeValue } from "./DateRange";
import { lastWeeks } from "./utils";

// There is no point in showing the period > 6 months. We prune stats older than
// 6 months.
export const numberOfWeeksOptions = [4, 12, 24] as const;

interface WeekPickerProps {
	value: DateRangeValue;
	onChange: (value: DateRangeValue) => void;
}

export const WeekPicker: FC<WeekPickerProps> = ({ value, onChange }) => {
	const anchorRef = useRef<HTMLButtonElement>(null);
	const [open, setOpen] = useState(false);
	const numberOfWeeks = dayjs(value.endDate).diff(
		dayjs(value.startDate),
		"week",
	);

	const handleClose = () => {
		setOpen(false);
	};

	return (
		<div>
			<Button
				variant="outline"
				ref={anchorRef}
				id="interval-button"
				aria-controls={open ? "interval-menu" : undefined}
				aria-haspopup="true"
				aria-expanded={open ? "true" : undefined}
				onClick={() => setOpen(true)}
			>
				Last {numberOfWeeks} weeks
				<ChevronDownIcon />
			</Button>
			<Menu
				id="interval-menu"
				anchorEl={anchorRef.current}
				open={open}
				onClose={handleClose}
				MenuListProps={{
					"aria-labelledby": "interval-button",
				}}
				anchorOrigin={{
					vertical: "bottom",
					horizontal: "left",
				}}
				transformOrigin={{
					vertical: "top",
					horizontal: "left",
				}}
			>
				{numberOfWeeksOptions.map((option) => {
					const optionRange = lastWeeks(option);

					return (
						<MenuItem
							css={{ fontSize: 14 }}
							className="text-sm justify-between leading-normal"
							key={option}
							onClick={() => {
								onChange(optionRange);
								handleClose();
							}}
						>
							Last {option} weeks
							<div className="size-4">
								{numberOfWeeks === option && (
									<CheckIcon className="size-icon-xs" />
								)}
							</div>
						</MenuItem>
					);
				})}
			</Menu>
		</div>
	);
};
