import { cva, type VariantProps } from "class-variance-authority";
import { Command as CommandPrimitive } from "cmdk";
import { cn } from "cn";
import { SearchIcon } from "lucide-react";

const commandVariants = cva(
	"flex h-full w-full flex-col overflow-hidden rounded-md bg-surface-primary text-content-primary",
	{
		variants: {
			size: {
				default: "",
				sm: `
 [&_[cmdk-input-wrapper]]:border-0 [&_[cmdk-input-wrapper]]:border-b [&_[cmdk-input-wrapper]]:border-solid [&_[cmdk-input-wrapper]]:border-border
 [&_[cmdk-input-wrapper]]:h-8 [&_[cmdk-input-wrapper]]:px-3 [&_[cmdk-input-wrapper]]:py-0 [&_[cmdk-input-wrapper]>svg]:size-3.5
 [&_[cmdk-input]]:h-full [&_[cmdk-input]]:p-0 [&_[cmdk-input]]:text-xs [&_[cmdk-input]]:leading-[18px]
 [&_[cmdk-list]]:p-2 [&_[cmdk-group]]:p-0 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:leading-[18px]
 [&_[cmdk-item]]:min-h-8 [&_[cmdk-item]]:gap-2 [&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-1.5 [&_[cmdk-item]]:text-xs [&_[cmdk-item]]:leading-[18px]
 [&_[cmdk-empty]]:py-3 [&_[cmdk-empty]]:text-xs [&_[cmdk-empty]]:leading-[18px]
 `,
			},
		},
		defaultVariants: { size: "default" },
	},
);

type CommandProps = React.ComponentProps<typeof CommandPrimitive> &
	VariantProps<typeof commandVariants>;

export const Command: React.FC<CommandProps> = ({
	className,
	size,
	...props
}) => {
	return (
		<CommandPrimitive
			className={cn(commandVariants({ size }), className)}
			{...props}
		/>
	);
};

export const CommandInput: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Input>
> = ({ className, ...props }) => {
	return (
		<div className="flex items-center border-b px-3" cmdk-input-wrapper="">
			<SearchIcon className="mr-2 size-4 shrink-0 opacity-50" />
			<CommandPrimitive.Input
				className={cn(
					`flex h-10 w-full rounded-md bg-transparent py-3 text-sm outline-hidden border-none
				placeholder:text-content-secondary text-content-primary
				disabled:cursor-not-allowed disabled:opacity-50`,
					className,
				)}
				{...props}
			/>
		</div>
	);
};

export const CommandList: React.FC<
	React.ComponentProps<typeof CommandPrimitive.List>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.List
			className={cn("max-h-96 overflow-y-auto overflow-x-hidden", className)}
			{...props}
		/>
	);
};

export const CommandEmpty: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Empty>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Empty
			className={cn("py-6 text-center text-sm", className)}
			{...props}
		/>
	);
};

export const CommandGroup: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Group>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Group
			className={cn(
				`overflow-hidden p-2 text-content-primary
			[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs
			[&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-content-secondary`,
				className,
			)}
			{...props}
		/>
	);
};

export const CommandSeparator: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Separator
			className={cn("-mx-1 h-px bg-border", className)}
			{...props}
		/>
	);
};

export const CommandItem: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Item>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Item
			className={cn(
				`relative flex cursor-default gap-2 select-none text-content-secondary items-center rounded-sm px-2 py-2 text-sm font-medium outline-hidden
			data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50
			data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary
			data-[selected=true]:ring-2 data-[selected=true]:ring-inset data-[selected=true]:ring-content-link
			[&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0`,
				className,
			)}
			{...props}
		/>
	);
};
