import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import { Button } from "components/Button/Button";
import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { type FC, useRef, useState } from "react";

const insightsIntervals = {
	day: {
		label: "Daily",
	},
	week: {
		label: "Weekly",
	},
} as const;

export type InsightsInterval = keyof typeof insightsIntervals;

interface IntervalMenuProps {
	value: InsightsInterval;
	onChange: (value: InsightsInterval) => void;
}

export const IntervalMenu: FC<IntervalMenuProps> = ({ value, onChange }) => {
	const anchorRef = useRef<HTMLButtonElement>(null);
	const [open, setOpen] = useState(false);

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
				variant="outline"
			>
				{insightsIntervals[value].label}
				<ChevronDownIcon className="size-icon-xs ml-1" />
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
				{Object.entries(insightsIntervals).map(([interval, { label }]) => {
					return (
						<MenuItem
							className="text-sm leading-none justify-between"
							key={interval}
							onClick={() => {
								onChange(interval as InsightsInterval);
								handleClose();
							}}
						>
							{label}
							<div className="size-4">
								{value === interval && <CheckIcon className="size-icon-xs" />}
							</div>
						</MenuItem>
					);
				})}
			</Menu>
		</div>
	);
};
