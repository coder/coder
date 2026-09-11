import { cn } from "cn";
import {
	ArrowLeftIcon,
	BotIcon,
	CheckIcon,
	ChevronRightIcon,
	PiIcon,
} from "lucide-react";
import { useState } from "react";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";

export type Harness = "Coder Agents" | "Codex" | "Claude Code" | "Pi";
const harnesses: readonly Harness[] = [
	"Coder Agents",
	"Codex",
	"Claude Code",
	"Pi",
];

/** Icon shared by the harness choices and the composer tile. */
export const HarnessIcon = ({ harness }: { harness: Harness }) => {
	if (harness === "Codex" || harness === "Claude Code") {
		return (
			<ExternalImage
				src={
					harness === "Codex" ? "/icon/openai-codex.svg" : "/icon/claude.svg"
				}
				alt=""
				className="size-3.5 shrink-0"
			/>
		);
	}
	const Icon = harness === "Pi" ? PiIcon : BotIcon;
	return <Icon className="size-3.5 shrink-0" aria-hidden />;
};

/** Frontend prototype choices; selecting a harness does not configure a backend. */
export const HarnessPickerList = ({
	value,
	onChange,
	onBack,
}: {
	value: Harness;
	onChange: (value: Harness) => void;
	onBack?: () => void;
}) => (
	<div>
		{onBack && (
			<>
				<button
					type="button"
					onClick={onBack}
					className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
				>
					<ArrowLeftIcon className="size-3.5 shrink-0" />
					Back
				</button>
				<Separator className="my-1" />
			</>
		)}
		<Command defaultValue={value} loop>
			<CommandInput
				placeholder="Search harnesses..."
				aria-label="Search harnesses"
				className="text-xs"
			/>
			<CommandList label="Harnesses">
				<CommandEmpty className="text-xs">No harnesses found</CommandEmpty>
				<CommandGroup>
					{harnesses.map((harness) => (
						<CommandItem
							key={harness}
							value={harness}
							onSelect={() => onChange(harness)}
							className="text-xs font-normal"
						>
							<HarnessIcon harness={harness} />
							<span>{harness}</span>
							{value === harness && (
								<CheckIcon className="ml-auto size-icon-sm shrink-0" />
							)}
						</CommandItem>
					))}
				</CommandGroup>
			</CommandList>
		</Command>
	</div>
);

/** Nested popover matching the desktop workspace picker. */
export const HarnessPicker = ({
	value,
	onChange,
}: {
	value: Harness;
	onChange: (value: Harness) => void;
}) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
				>
					<HarnessIcon harness={value} />
					<span>Change harness</span>
					<ChevronRightIcon
						className={cn(
							"ml-auto size-icon-sm transition-transform",
							open && "rotate-180",
						)}
					/>
				</button>
			</PopoverTrigger>
			<PopoverContent
				side="right"
				align="start"
				sideOffset={8}
				className="w-64 p-0"
			>
				<HarnessPickerList
					value={value}
					onChange={(harness) => {
						setOpen(false);
						onChange(harness);
					}}
				/>
			</PopoverContent>
		</Popover>
	);
};
