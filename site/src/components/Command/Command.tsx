import { Command as CommandPrimitive } from "cmdk";
import { SearchIcon } from "lucide-react";
import { cn } from "#/utils/cn";

export const Command: React.FC<
	React.ComponentPropsWithRef<typeof CommandPrimitive>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive
			className={cn(
				"flex h-full w-full flex-col overflow-hidden rounded-md bg-surface-primary text-content-primary",
				className,
			)}
			{...props}
		/>
	);
};

export const CommandInput: React.FC<
	React.ComponentPropsWithRef<typeof CommandPrimitive.Input>
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
	React.ComponentPropsWithRef<typeof CommandPrimitive.List>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.List
			className={cn("max-h-96 overflow-y-auto overflow-x-hidden", className)}
			{...props}
		/>
	);
};

export const CommandEmpty: React.FC<
	React.ComponentPropsWithRef<typeof CommandPrimitive.Empty>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Empty
			className={cn("py-6 text-center text-sm", className)}
			{...props}
		/>
	);
};

export const CommandGroup: React.FC<
	React.ComponentPropsWithRef<typeof CommandPrimitive.Group>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Group
			className={cn(
				`overflow-hidden p-2 text-content-primary
			**:[[cmdk-group-heading]]:px-2 **:[[cmdk-group-heading]]:py-1.5 **:[[cmdk-group-heading]]:text-xs
			**:[[cmdk-group-heading]]:font-medium **:[[cmdk-group-heading]]:text-content-secondary`,
				className,
			)}
			{...props}
		/>
	);
};

export const CommandSeparator: React.FC<
	React.ComponentPropsWithRef<typeof CommandPrimitive.Separator>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Separator
			className={cn("-mx-1 h-px bg-border", className)}
			{...props}
		/>
	);
};

export const CommandItem: React.FC<
	React.ComponentPropsWithRef<typeof CommandPrimitive.Item>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Item
			className={cn(
				`relative flex cursor-default gap-2 select-none text-content-secondary items-center rounded-sm px-2 py-2 text-sm font-medium outline-hidden
			data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50
			data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary
			[&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0`,
				className,
			)}
			{...props}
		/>
	);
};
