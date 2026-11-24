import { Command as CommandPrimitive } from "cmdk";
import { cn } from "cn";
import { SearchIcon } from "lucide-react";
import { createContext, useCallback, useContext, useState } from "react";
import {
	keyboardHighlightRingClassName,
	useHighlightSource,
} from "./useHighlightSource";

/**
 * Returns an event handler that runs the component's internal handler and then
 * the consumer's handler from props, if any, so consumers can still observe
 * the event without replacing the internal behavior.
 */
const composeEventHandlers =
	<E,>(
		internalHandler: (event: E) => void,
		externalHandler: ((event: E) => void) | undefined,
	) =>
	(event: E) => {
		internalHandler(event);
		externalHandler?.(event);
	};

const CommandStatusContext = createContext<HTMLElement | null>(null);

export const Command: React.FC<
	React.ComponentProps<typeof CommandPrimitive>
> = ({ className, onFocus, onPointerMove, children, ...props }) => {
	const highlightSource = useHighlightSource();
	const [statusNode, setStatusNode] = useState<HTMLElement | null>(null);
	return (
		<CommandPrimitive
			className={cn(
				"flex size-full flex-col overflow-hidden rounded-md bg-surface-primary text-content-primary",
				className,
			)}
			data-highlight-source={highlightSource.source}
			onFocus={composeEventHandlers(
				highlightSource.setToLastPageInput,
				onFocus,
			)}
			onPointerMove={composeEventHandlers(
				highlightSource.setToPointer,
				onPointerMove,
			)}
			{...props}
		>
			<CommandStatusContext.Provider value={statusNode}>
				{children}
			</CommandStatusContext.Provider>
			{/* Screen readers only announce live regions that exist before their
			    text changes, and a status is not allowed inside the listbox. */}
			<span ref={setStatusNode} role="status" className="sr-only" />
		</CommandPrimitive>
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
					"flex h-10 w-full rounded-md bg-transparent py-3 text-sm outline-hidden border-none",
					"placeholder:text-content-secondary text-content-primary",
					"disabled:cursor-not-allowed disabled:opacity-50",
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
	Omit<React.ComponentProps<typeof CommandPrimitive.Empty>, "ref">
> = ({ className, ...props }) => {
	const statusNode = useContext(CommandStatusContext);
	// Copies text only, so links in the message never get a hidden duplicate.
	// Stable so re-renders do not clear and rewrite the live region.
	const mirrorToStatus = useCallback(
		(node: HTMLDivElement | null) => {
			if (!node || !statusNode) {
				return;
			}
			const sync = () => {
				statusNode.textContent = node.textContent;
			};
			sync();
			const observer = new MutationObserver(sync);
			observer.observe(node, {
				childList: true,
				characterData: true,
				subtree: true,
			});
			return () => {
				observer.disconnect();
				statusNode.textContent = "";
			};
		},
		[statusNode],
	);
	return (
		<CommandPrimitive.Empty
			className={cn("py-6 text-center text-sm", className)}
			{...props}
			ref={mirrorToStatus}
		/>
	);
};

export const CommandGroup: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Group>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Group
			className={cn(
				"overflow-hidden p-2 text-content-primary",
				"**:[[cmdk-group-heading]]:px-2 **:[[cmdk-group-heading]]:py-1.5 **:[[cmdk-group-heading]]:text-xs",
				"**:[[cmdk-group-heading]]:font-medium **:[[cmdk-group-heading]]:text-content-secondary",
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

/**
 * A cmdk row. Pass `aria-selected` to mark the row holding the chosen value;
 * otherwise cmdk marks the highlighted row as selected.
 */
export const CommandItem: React.FC<
	React.ComponentProps<typeof CommandPrimitive.Item>
> = ({ className, ...props }) => {
	return (
		<CommandPrimitive.Item
			className={cn(
				"relative flex cursor-default gap-2 select-none text-content-secondary items-center rounded-sm px-2 py-2 text-sm font-medium outline-hidden",
				"data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50",
				"data-[selected=true]:bg-surface-secondary data-[selected=true]:text-content-primary",
				"[&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
				keyboardHighlightRingClassName,
				className,
			)}
			{...props}
		/>
	);
};
