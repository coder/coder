/**
 * Copied from shadc/ui on 04/04/2025
 * @see {@link https://ui.shadcn.com/docs/components/radio-group}
 */
import * as RadioGroupPrimitive from "@radix-ui/react-radio-group";
import { Circle } from "lucide-react";
import { cn } from "utils/cn";

export const RadioGroup: React.FC<
	React.ComponentPropsWithRef<typeof RadioGroupPrimitive.Root>
> = ({ className, ...props }) => {
	return (
		<RadioGroupPrimitive.Root
			className={cn("grid gap-2", className)}
			{...props}
		/>
	);
};

export const RadioGroupItem: React.FC<
	React.ComponentPropsWithRef<typeof RadioGroupPrimitive.Item>
> = ({ className, ...props }) => {
	return (
		<RadioGroupPrimitive.Item
			className={cn(
				`focus:outline-none focus-visible:ring-2 focus-visible:ring-content-link
				focus-visible:ring-offset-4 focus-visible:ring-offset-surface-primary
				disabled:cursor-not-allowed disabled:opacity-25 disabled:border-surface-invert-primary
				hover:border-border-hover data-[state=checked]:border-border-hover`,
				className,
			)}
			{...props}
		>
			<RadioGroupPrimitive.Indicator className="flex items-center justify-center">
				<Circle className="absolute h-2.5 w-2.5 fill-surface-invert-primary" />
			</RadioGroupPrimitive.Indicator>
		</RadioGroupPrimitive.Item>
	);
};
