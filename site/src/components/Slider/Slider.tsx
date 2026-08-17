/**
 * Copied from shadc/ui on 04/16/2025
 * @see {@link https://ui.shadcn.com/docs/components/slider}
 */
import { Slider as SliderPrimitive } from "radix-ui";
import { cn } from "#/utils/cn";

export const Slider: React.FC<
	React.ComponentPropsWithRef<typeof SliderPrimitive.Root>
> = ({ className, defaultValue, value, min = 0, max = 100, ...props }) => {
	const thumbCount = Array.isArray(value)
		? value.length
		: Array.isArray(defaultValue)
			? defaultValue.length
			: 1;

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
			{...props}
		>
			<SliderPrimitive.Track className="relative h-2 w-full grow overflow-hidden rounded-full bg-surface-secondary data-[disabled]:opacity-40">
				<SliderPrimitive.Range className="absolute h-full bg-content-primary" />
			</SliderPrimitive.Track>
			{Array.from({ length: thumbCount }, (_, index) => (
				<SliderPrimitive.Thumb
					key={`slider-thumb-${index}`}
					className="block size-4 rounded-full border border-solid border-surface-invert-secondary bg-surface-primary shadow transition-colors
			focus-visible:outline-none hover:border-content-primary
			focus-visible:ring-0 focus-visible:ring-content-primary focus-visible:ring-offset-surface-primary
			disabled:pointer-events-none data-[disabled]:opacity-100 data-[disabled]:border-border"
				/>
			))}
		</SliderPrimitive.Root>
	);
};
