/**
 * Copied from shadc/ui on 11/13/2024
 * @see {@link https://ui.shadcn.com/docs/components/command}
 */
import type { DialogProps } from "@radix-ui/react-dialog";
import { Command as CommandPrimitive } from "cmdk";
import { Dialog, DialogContent } from "components/Dialog/Dialog";
import { Search } from "lucide-react";
import { type FC, forwardRef } from "react";
import { cn } from "utils/cn";

export const Command = forwardRef<
	React.ElementRef<typeof CommandPrimitive>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive>
>(({ className, ...props }, ref) => (
	<CommandPrimitive
		ref={ref}
		className={cn(
			"flex h-full w-full flex-col overflow-hidden rounded-md bg-surface-primary text-content-primary",
			className,
		)}
		{...props}
	/>
));

const CommandDialog: FC<DialogProps> = ({ children, ...props }) => {
	return (
		<Dialog {...props}>
			<DialogContent className="overflow-hidden p-0">
				<Command
					className={`
						[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-content-disabled
						[&_[cmdk-group]]:px-2 [&_[cmdk-group]:not([hidden])_~[cmdk-group]]:pt-0
						[&_[cmdk-input-wrapper]_svg]:h-5 [&_[cmdk-input-wrapper]_svg]:w-5
						[&_[cmdk-input]]:h-12
						[&_[cmdk-item]]:px-2 [&_[cmdk-item]]:py-3
						[&_[cmdk-item]_svg]:h-5 [&_[cmdk-item]_svg]:w-5
					`}
				>
					{children}
				</Command>
			</DialogContent>
		</Dialog>
	);
};

export const CommandInput = forwardRef<
	React.ElementRef<typeof CommandPrimitive.Input>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive.Input>
>(({ className, ...props }, ref) => (
	<div className="flex items-center border-b px-3" cmdk-input-wrapper="">
		<Search className="mr-2 h-4 w-4 shrink-0 opacity-50" />
		<CommandPrimitive.Input
			ref={ref}
			className={cn(
				`flex h-10 w-full rounded-md bg-transparent py-3 text-sm outline-none border-none
				placeholder:text-content-secondary
				disabled:cursor-not-allowed disabled:opacity-50`,
				className,
			)}
			{...props}
		/>
	</div>
));

export const CommandList = forwardRef<
	React.ElementRef<typeof CommandPrimitive.List>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive.List>
>(({ className, ...props }, ref) => (
	<CommandPrimitive.List
		ref={ref}
		className={cn(
			"max-h-96 overflow-y-auto overflow-x-hidden border-0 border-t border-solid border-border",
			className,
		)}
		{...props}
	/>
));

export const CommandEmpty = forwardRef<
	React.ElementRef<typeof CommandPrimitive.Empty>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive.Empty>
>((props, ref) => (
	<CommandPrimitive.Empty
		ref={ref}
		className="py-6 text-center text-sm"
		{...props}
	/>
));

export const CommandGroup = forwardRef<
	React.ElementRef<typeof CommandPrimitive.Group>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive.Group>
>(({ className, ...props }, ref) => (
	<CommandPrimitive.Group
		ref={ref}
		className={cn(
			`overflow-hidden p-2 text-content-primary
			[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs
			[&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-content-secondary`,
			className,
		)}
		{...props}
	/>
));

export const CommandSeparator = forwardRef<
	React.ElementRef<typeof CommandPrimitive.Separator>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive.Separator>
>(({ className, ...props }, ref) => (
	<CommandPrimitive.Separator
		ref={ref}
		className={cn("-mx-1 h-px bg-border", className)}
		{...props}
	/>
));

export const CommandItem = forwardRef<
	React.ElementRef<typeof CommandPrimitive.Item>,
	React.ComponentPropsWithoutRef<typeof CommandPrimitive.Item>
>(({ className, ...props }, ref) => (
	<CommandPrimitive.Item
		ref={ref}
		className={cn(
			`relative flex cursor-default gap-2 select-none text-content-secondary items-center rounded-sm px-2 py-2 text-sm font-medium outline-none
			data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50
			data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary
			[&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0`,
			className,
		)}
		{...props}
	/>
));

const CommandShortcut = ({
	className,
	...props
}: React.HTMLAttributes<HTMLSpanElement>) => {
	return (
		<span
			className={cn(
				"ml-auto text-xs tracking-widest text-content-disabled",
				className,
			)}
			{...props}
		/>
	);
};
