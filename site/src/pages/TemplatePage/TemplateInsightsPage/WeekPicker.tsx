import Button from "@mui/material/Button";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import dayjs from "dayjs";
import { ChevronDownIcon } from "lucide-react";
import { CheckIcon } from "lucide-react";
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
				ref={anchorRef}
				id="interval-button"
				aria-controls={open ? "interval-menu" : undefined}
				aria-haspopup="true"
				aria-expanded={open ? "true" : undefined}
				onClick={() => setOpen(true)}
				endIcon={<ChevronDownIcon className="size-icon-xs" />}
			>
				Last {numberOfWeeks} weeks
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
							css={{ fontSize: 14, justifyContent: "space-between" }}
							key={option}
							onClick={() => {
								onChange(optionRange);
								handleClose();
							}}
						>
							Last {option} weeks
							<div css={{ width: 16, height: 16 }}>
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
