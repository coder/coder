import { cn } from "cn";
import {
	ArrowLeftIcon,
	CheckIcon,
	ChevronRightIcon,
	RotateCcwIcon,
} from "lucide-react";
import { useId, useState } from "react";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { Input } from "#/components/Input/Input";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Separator } from "#/components/Separator/Separator";
import { isBelowMdViewport } from "#/utils/mobile";
import type { Harness } from "./HarnessPicker";
import { harnessConfig } from "./harnessConfig";

/** Harness-specific mock settings rendered inside the composer's plus menu. */
export const HarnessConfigMenu = ({
	harness,
	values,
	onChange,
}: {
	harness: Harness;
	values: Readonly<Record<string, string>>;
	onChange: (id: string, value: string) => void;
}) => {
	const directoryInputId = useId();
	const [directoryDraft, setDirectoryDraft] = useState("/home/coder");
	const [openId, setOpenId] = useState<string | null>(null);
	return (
		<>
			{harness !== "Coder Agents" && (
				<Popover
					open={openId === "working_directory"}
					onOpenChange={(open) => {
						if (open)
							setDirectoryDraft(values.working_directory || "/home/coder");
						setOpenId(open ? "working_directory" : null);
					}}
				>
					<PopoverTrigger asChild>
						<button
							type="button"
							className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
						>
							<span>Working directory</span>{" "}
							<span
								className="ml-auto max-w-32 truncate pl-4"
								title={values.working_directory || "/home/coder"}
							>
								{values.working_directory || "/home/coder"}
							</span>
							<ChevronRightIcon className="size-icon-sm shrink-0" />
						</button>
					</PopoverTrigger>
					<PopoverContent
						side="right"
						align="start"
						sideOffset={8}
						className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-72 p-3"
					>
						{isBelowMdViewport() && (
							<button
								type="button"
								onClick={() => setOpenId(null)}
								className="mb-3 flex cursor-pointer items-center gap-1.5 border-none bg-transparent p-0 text-xs text-content-secondary hover:text-content-primary"
							>
								<ArrowLeftIcon className="size-3.5" />
								Back
							</button>
						)}
						<label
							htmlFor={directoryInputId}
							className="mb-2 block text-xs text-content-primary"
						>
							Working directory
						</label>
						<div className="relative">
							<Input
								id={directoryInputId}
								value={directoryDraft}
								onChange={(event) =>
									setDirectoryDraft(event.currentTarget.value)
								}
								placeholder="/home/coder"
								className="h-8 pr-9 text-xs md:text-xs"
								onKeyDown={(event) => {
									if (event.key === "Enter") {
										event.preventDefault();
										onChange(
											"working_directory",
											directoryDraft.trim() || "/home/coder",
										);
										setOpenId(null);
									}
								}}
							/>
							<Button
								type="button"
								variant="subtle"
								size="icon"
								className="absolute right-0 top-0"
								aria-label="Reset working directory to default"
								title="Reset to default"
								onClick={() => setDirectoryDraft("/home/coder")}
							>
								<RotateCcwIcon />
							</Button>
						</div>
						<div className="mt-3 flex justify-end gap-2">
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={() => setOpenId(null)}
							>
								Cancel
							</Button>
							<Button
								type="button"
								size="sm"
								onClick={() => {
									onChange(
										"working_directory",
										directoryDraft.trim() || "/home/coder",
									);
									setOpenId(null);
								}}
							>
								Save
							</Button>
						</div>
					</PopoverContent>
				</Popover>
			)}
			{harnessConfig[harness].map((config) => {
				const value = values[config.id] ?? config.currentValue;
				const selected = config.options.find(
					(option) => option.value === value,
				);
				const open = openId === config.id;
				return (
					<Popover
						key={config.id}
						open={open}
						onOpenChange={(next) => setOpenId(next ? config.id : null)}
					>
						<PopoverTrigger asChild>
							<button
								type="button"
								className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary"
							>
								<span>{config.name}</span>{" "}
								<span className="ml-auto max-w-32 truncate pl-4 text-content-secondary">
									{selected?.name ?? value}
								</span>
								<ChevronRightIcon
									className={cn(
										"size-icon-sm shrink-0 transition-transform",
										open && "rotate-180",
									)}
								/>
							</button>
						</PopoverTrigger>
						<PopoverContent
							side="right"
							align="start"
							sideOffset={8}
							className="mobile-full-width-dropdown mobile-full-width-dropdown-bottom w-72 p-0"
						>
							{isBelowMdViewport() && (
								<>
									<button
										type="button"
										onClick={() => setOpenId(null)}
										className="flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-2 text-xs text-content-secondary hover:text-content-primary"
									>
										<ArrowLeftIcon className="size-3.5" />
										Back
									</button>
									<Separator />
								</>
							)}
							<Command defaultValue={value} loop>
								<CommandInput
									placeholder={`Search ${config.name.toLowerCase()}...`}
									aria-label={`Search ${config.name.toLowerCase()}`}
									className="text-xs"
								/>
								<CommandList label={config.name}>
									<CommandEmpty className="text-xs">
										No options found
									</CommandEmpty>
									<CommandGroup heading={config.description}>
										{config.options.map((option) => (
											<CommandItem
												key={option.value}
												value={option.value}
												keywords={[option.name]}
												onSelect={() => {
													onChange(config.id, option.value);
													setOpenId(null);
												}}
												className="text-xs font-normal"
											>
												<span className="flex min-w-0 flex-1 flex-col gap-1">
													<span>{option.name}</span>{" "}
													{option.description && (
														<span className="text-xs leading-relaxed text-content-secondary">
															{option.description}
														</span>
													)}
												</span>
												{value === option.value && (
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
			})}
		</>
	);
};
