import { cn } from "cn";
import { type FC, useState } from "react";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { CARD_COLORS, type CardColor } from "./boardLabels";
import { cardSwatch } from "./cardColor";

interface CardColorPickerProps {
	/** The card title, for the stripe's accessible name. */
	readonly title: string;
	readonly value: CardColor | undefined;
	readonly onChange: (color: CardColor | undefined) => void;
}

/**
 * The card's left stripe as a button: the stripe shows the color, so the
 * stripe is where you change it. The swatches float beside it rather than
 * reflowing the header.
 */
export const CardColorPicker: FC<CardColorPickerProps> = ({
	title,
	value,
	onChange,
}) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					title="Card color"
					aria-label={`Color of ${title}`}
					className="absolute inset-y-0 left-0 z-[1] w-2 border-0 bg-transparent p-0 hover:bg-content-primary/10 data-[state=open]:bg-content-primary/10"
				/>
			</PopoverTrigger>
			<PopoverContent
				side="right"
				align="start"
				sideOffset={6}
				className="flex w-auto items-center gap-1.5 p-2"
				onPointerDown={(e) => e.stopPropagation()}
			>
				<Swatch
					label="No color"
					selected={value === undefined}
					className="border-border bg-surface-primary"
					onClick={() => {
						setOpen(false);
						onChange(undefined);
					}}
				/>
				{CARD_COLORS.map((name) => (
					<Swatch
						key={name}
						label={name}
						selected={value === name}
						className={cn("border-transparent", cardSwatch({ color: name }))}
						onClick={() => {
							setOpen(false);
							onChange(name);
						}}
					/>
				))}
			</PopoverContent>
		</Popover>
	);
};

interface SwatchProps {
	readonly label: string;
	readonly selected: boolean;
	readonly className: string;
	readonly onClick: () => void;
}

const Swatch: FC<SwatchProps> = ({ label, selected, className, onClick }) => (
	<button
		type="button"
		aria-label={label}
		aria-pressed={selected}
		className={cn(
			"size-4 rounded-full border p-0 transition-transform hover:scale-110",
			className,
			selected &&
				"ring-2 ring-content-link ring-offset-1 ring-offset-surface-primary",
		)}
		onClick={onClick}
	/>
);
