import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ChevronRightIcon,
	LayersIcon,
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

/** Static template choices for the frontend-only workspace creation prototype. */
export const chatTemplateOptions = [
	{ id: "docker", name: "Docker", icon: "/icon/docker.svg" },
	{ id: "nodejs", name: "Node.js", icon: "/icon/nodejs.svg" },
	{ id: "python", name: "Python", icon: "/icon/python.svg" },
];

interface ChatTemplatePickerProps {
	value: string | null;
	onChange: (id: string) => void;
}

/** Template choices for starting a chat in a new workspace. */
export const ChatTemplatePickerList = ({
	value,
	onChange,
	onBack,
}: ChatTemplatePickerProps & { onBack?: () => void }) => (
	<>
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
		<Command loop>
			<CommandInput
				placeholder="Search templates..."
				aria-label="Search templates"
				className="text-xs"
			/>
			<CommandList label="Templates">
				<CommandEmpty className="text-xs">No templates found</CommandEmpty>
				<CommandGroup>
					{chatTemplateOptions.map((template) => (
						<CommandItem
							key={template.id}
							value={template.name}
							onSelect={() => onChange(template.id)}
							className="text-xs font-normal"
						>
							<ExternalImage
								src={template.icon}
								alt=""
								className="size-3.5 shrink-0"
							/>
							{template.name}
							{value === template.id && (
								<CheckIcon className="ml-auto size-icon-sm shrink-0" />
							)}
						</CommandItem>
					))}
				</CommandGroup>
			</CommandList>
		</Command>
		<p className="m-0 border-t border-solid px-3 py-2 text-xs leading-relaxed text-content-secondary">
			Start in a new workspace using this template.
		</p>
	</>
);

/** Desktop template picker matching the workspace attachment menu. */
export const ChatTemplatePicker = ({
	value,
	onChange,
}: ChatTemplatePickerProps) => {
	const [open, setOpen] = useState(false);
	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
				>
					<LayersIcon className="size-3.5 shrink-0" />
					<span>Attach template</span>
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
				<ChatTemplatePickerList
					value={value}
					onChange={(id) => {
						onChange(id);
						setOpen(false);
					}}
				/>
			</PopoverContent>
		</Popover>
	);
};
