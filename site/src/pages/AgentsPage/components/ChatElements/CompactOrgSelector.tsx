import { CheckIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { Organization } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

interface CompactOrgSelectorProps {
	value: Organization | null;
	onChange?: (organization: Organization) => void;
	options: readonly Organization[];
	disabled?: boolean;
	className?: string;
	dropdownSide?: "top" | "bottom" | "left" | "right";
	dropdownAlign?: "start" | "center" | "end";
}

export const CompactOrgSelector: FC<CompactOrgSelectorProps> = ({
	value,
	onChange,
	options,
	disabled = false,
	className,
	dropdownSide = "bottom",
	dropdownAlign = "start",
}) => {
	const [open, setOpen] = useState(false);
	const isDisabled = disabled || options.length === 0;

	return (
		<Popover open={open} onOpenChange={isDisabled ? undefined : setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					disabled={isDisabled}
					data-testid="compact-org-selector"
					aria-label={
						value
							? `Organization: ${value.display_name || value.name}`
							: "Select organization"
					}
					className={cn(
						"group flex h-6 w-auto cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none whitespace-nowrap transition-colors",
						"hover:text-content-primary focus:ring-0",
						"disabled:cursor-not-allowed disabled:opacity-50",
						className,
					)}
				>
					{value ? (
						<>
							<Avatar
								size="sm"
								src={value.icon}
								fallback={value.display_name || value.name}
								className="!size-3.5 border-0"
							/>
							<span className="truncate">
								{value.display_name || value.name}
							</span>
						</>
					) : (
						<span>Select org…</span>
					)}
					<ChevronDownIcon
						open={open}
						className="size-icon-sm shrink-0 text-content-secondary transition-colors hover:text-content-primary group-hover:text-content-primary"
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-64 p-0"
			>
				<Command loop>
					<CommandInput placeholder="Find organization…" className="text-xs" />
					<CommandList>
						<CommandEmpty className="text-xs">
							No organizations found
						</CommandEmpty>
						<CommandGroup>
							{options.map((org) => (
								<CommandItem
									className="text-xs font-normal"
									key={org.id}
									value={`${org.display_name} ${org.name}`}
									onSelect={() => {
										onChange?.(org);
										setOpen(false);
									}}
								>
									{" "}
									<Avatar
										size="sm"
										src={org.icon}
										fallback={org.display_name || org.name}
										className="!size-3.5 border-0"
									/>
									<span className="truncate">
										{org.display_name || org.name}
									</span>
									{value?.id === org.id && (
										<CheckIcon className="ml-auto size-icon-sm shrink-0" />
									)}
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
