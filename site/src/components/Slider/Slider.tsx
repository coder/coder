import { Slider as SliderPrimitive } from "@base-ui/react/slider";
import { cn } from "cn";

type SliderProps = Omit<
	SliderPrimitive.Root.Props<number[]>,
	"className" | "minStepsBetweenValues" | "onValueCommitted"
> & {
	className?: string;
	minStepsBetweenThumbs?: number;
	onValueCommit?: SliderPrimitive.Root.Props<number[]>["onValueCommitted"];
};

export const Slider: React.FC<SliderProps> = ({
	className,
	"aria-label": ariaLabel,
	"aria-labelledby": ariaLabelledBy,
	"aria-describedby": ariaDescribedBy,
	defaultValue = [0],
	value,
	min = 0,
	max = 100,
	minStepsBetweenThumbs = 0,
	onValueCommit,
	...props
}) => {
	const thumbCount = (value ?? defaultValue).length;

	return (
		<SliderPrimitive.Root
			className={cn(
				"relative flex w-full touch-none select-none items-center h-1.5",
				className,
			)}
			defaultValue={defaultValue}
			value={value}
			min={min}
			max={max}
			minStepsBetweenValues={minStepsBetweenThumbs}
			onValueCommitted={onValueCommit}
			thumbAlignment="edge"
			thumbCollisionBehavior="none"
			{...props}
		>
			<SliderPrimitive.Control className="relative flex h-full w-full items-center">
				<SliderPrimitive.Track className="relative h-2 w-full grow overflow-hidden rounded-full bg-surface-secondary data-disabled:opacity-40">
					<SliderPrimitive.Indicator className="absolute h-full bg-content-primary" />
				</SliderPrimitive.Track>
				{Array.from({ length: thumbCount }, (_, index) => (
					<SliderPrimitive.Thumb
						key={`slider-thumb-${index}`}
						index={index}
						aria-label={ariaLabel}
						aria-labelledby={ariaLabelledBy}
						aria-describedby={ariaDescribedBy}
						className="block size-4 rounded-full border border-solid border-surface-invert-secondary bg-surface-primary shadow transition-colors
			has-focus-visible:outline-hidden hover:border-content-primary
			has-focus-visible:ring-0 has-focus-visible:ring-content-primary has-focus-visible:ring-offset-surface-primary
			data-disabled:pointer-events-none data-disabled:opacity-100 data-disabled:border-border"
					/>
				))}
			</SliderPrimitive.Control>
		</SliderPrimitive.Root>
	);
};
