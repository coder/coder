import { Radio } from "@base-ui/react/radio";
import { RadioGroup as RadioGroupPrimitive } from "@base-ui/react/radio-group";
import { cn } from "cn";
import { CircleIcon } from "lucide-react";

type RadioGroupProps = Omit<
	RadioGroupPrimitive.Props<string>,
	"className" | "onValueChange"
> & { onValueChange?: (value: string) => void; className?: string };

export const RadioGroup: React.FC<RadioGroupProps> = ({
	className,
	onValueChange,
	...props
}) => {
	return (
		<RadioGroupPrimitive
			className={cn("grid gap-2", className)}
			onValueChange={(value) => onValueChange?.(value)}
			{...props}
		/>
	);
};

type RadioGroupItemProps = Omit<Radio.Root.Props<string>, "className"> & {
	className?: string;
};

export const RadioGroupItem: React.FC<RadioGroupItemProps> = ({
	className,
	...props
}) => {
	return (
		<Radio.Root
			nativeButton
			render={(buttonProps) => <button {...buttonProps} type="button" />}
			className={cn(
				`relative aspect-square size-4 rounded-full border border-solid border-border-control text-content-primary bg-surface-primary
				focus:outline-hidden focus-visible:ring-2 focus-visible:ring-content-link
				focus-visible:ring-offset-4 focus-visible:ring-offset-surface-primary
				disabled:cursor-not-allowed disabled:opacity-25 disabled:border-surface-invert-primary
				hover:border-border-control-hover`,
				className,
			)}
			{...props}
		>
			<Radio.Indicator className="flex items-center justify-center">
				<CircleIcon className="absolute h-2.5 w-2.5 fill-surface-invert-primary" />
			</Radio.Indicator>
		</Radio.Root>
	);
};
