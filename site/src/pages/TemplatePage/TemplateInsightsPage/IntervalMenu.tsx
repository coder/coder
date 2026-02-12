import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuRadioGroup,
	DropdownMenuRadioItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { ChevronDownIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

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
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button variant="outline" className="group">
					{insightsIntervals[value].label}
					<ChevronDownIcon
						className={cn(
							"group-data-[state=open]:rotate-180 transition-transform",
						)}
					/>
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start">
				<DropdownMenuRadioGroup
					value={value}
					onValueChange={(v) => onChange(v as InsightsInterval)}
				>
					{Object.entries(insightsIntervals).map(([interval, { label }]) => (
						<DropdownMenuRadioItem key={interval} value={interval}>
							{label}
						</DropdownMenuRadioItem>
					))}
				</DropdownMenuRadioGroup>
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
