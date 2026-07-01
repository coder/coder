import { useLayoutEffect, useState } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import {
	Popover,
	PopoverAnchor,
	PopoverContent,
} from "#/components/Popover/Popover";
import type { BuiltInSlashCommand } from "../../utils/builtInSlashCommands";
import { personalSkillTriggerText } from "../../utils/personalSkills";

// Prevent zero-height anchors when the browser returns a degenerate caret rect.
const MIN_ANCHOR_HEIGHT_PX = 16;

export type CaretAnchorRect = {
	top: number;
	left: number;
	height: number;
};

type PersonalSkillsTriggerMenuProps = {
	open: boolean;
	anchorRect: CaretAnchorRect | null;
	query: string;
	builtInCommands: readonly BuiltInSlashCommand[];
	skills: readonly TypesGen.UserSkillMetadata[];
	isLoading?: boolean;
	isError?: boolean;
	selectedIndex: number;
	onSelectedIndexChange: (index: number) => void;
	onCommandSelect: (command: BuiltInSlashCommand) => void;
	onSelect: (skill: TypesGen.UserSkillMetadata) => void;
	onClose: () => void;
};

type PersonalSkillsMenuState = {
	anchorRect: CaretAnchorRect;
	query: string;
	builtInCommands: readonly BuiltInSlashCommand[];
	skills: readonly TypesGen.UserSkillMetadata[];
	isLoading?: boolean;
	isError?: boolean;
	selectedIndex: number;
};

export const PersonalSkillsTriggerMenu = ({
	open,
	anchorRect,
	query,
	builtInCommands,
	skills,
	isLoading,
	isError,
	selectedIndex,
	onSelectedIndexChange,
	onCommandSelect,
	onSelect,
	onClose,
}: PersonalSkillsTriggerMenuProps) => {
	const [lastOpenMenuState, setLastOpenMenuState] =
		useState<PersonalSkillsMenuState | null>(null);
	const isAnchoredOpen = open && anchorRect !== null;
	const activeMenuState: PersonalSkillsMenuState | null = isAnchoredOpen
		? {
				anchorRect,
				query,
				builtInCommands,
				skills,
				isLoading,
				isError,
				selectedIndex,
			}
		: null;
	const menuState = activeMenuState ?? lastOpenMenuState;
	const menuAnchorRect = menuState?.anchorRect ?? null;
	const menuBuiltInCommands = menuState?.builtInCommands ?? [];
	const menuSkills = menuState?.skills ?? [];
	const menuSelectedIndex = menuState?.selectedIndex ?? -1;
	const menuItemCount = menuBuiltInCommands.length + menuSkills.length;
	const selectedValue =
		menuSelectedIndex < 0
			? ""
			: menuSelectedIndex < menuBuiltInCommands.length
				? `command:${menuBuiltInCommands[menuSelectedIndex]?.id}`
				: `skill:${menuSkills[menuSelectedIndex - menuBuiltInCommands.length]?.id}`;

	useLayoutEffect(() => {
		if (!isAnchoredOpen) {
			return;
		}
		setLastOpenMenuState({
			anchorRect,
			builtInCommands,
			query,
			skills,
			isLoading,
			isError,
			selectedIndex,
		});
	}, [
		anchorRect,
		builtInCommands,
		isAnchoredOpen,
		isError,
		isLoading,
		query,
		selectedIndex,
		skills,
	]);

	const handleHighlightedValueChange = (value: string) => {
		const commandIndex = menuBuiltInCommands.findIndex(
			(command) => value === `command:${command.id}`,
		);
		let nextIndex = commandIndex;
		if (nextIndex < 0) {
			const skillIndex = menuSkills.findIndex(
				(skill) => value === `skill:${skill.id}`,
			);
			if (skillIndex >= 0) {
				nextIndex = skillIndex + menuBuiltInCommands.length;
			}
		}
		if (nextIndex >= 0) {
			onSelectedIndexChange(nextIndex);
		}
	};

	return (
		<Popover
			open={isAnchoredOpen}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onClose();
				}
			}}
		>
			{menuAnchorRect && (
				<PopoverAnchor asChild>
					<span
						aria-hidden="true"
						style={{
							position: "fixed",
							top: menuAnchorRect.top,
							left: menuAnchorRect.left,
							width: 1,
							height: Math.max(menuAnchorRect.height, MIN_ANCHOR_HEIGHT_PX),
							pointerEvents: "none",
						}}
					/>
				</PopoverAnchor>
			)}
			<PopoverContent
				align="start"
				side="bottom"
				className="w-80 overflow-hidden p-1 mobile-full-width-dropdown mobile-full-width-dropdown-above-composer"
				onMouseDown={(event) => event.preventDefault()}
				onOpenAutoFocus={(event) => event.preventDefault()}
				onCloseAutoFocus={(event) => event.preventDefault()}
			>
				<Command
					shouldFilter={false}
					loop={false}
					onValueChange={handleHighlightedValueChange}
					value={selectedValue}
				>
					<CommandList className="max-h-72 border-t-0 mobile-full-width-dropdown-scroll-area">
						{menuState?.isLoading && menuItemCount === 0 ? (
							<CommandItem value="loading" disabled>
								Loading personal skills...
							</CommandItem>
						) : menuState?.isError && menuItemCount === 0 ? (
							<CommandItem value="error" disabled>
								Could not load personal skills. Close and type / again to retry.
							</CommandItem>
						) : menuItemCount === 0 ? (
							<CommandEmpty>
								{menuState?.query
									? "No slash commands or personal skills match that query."
									: "No slash commands or personal skills found."}
							</CommandEmpty>
						) : (
							<>
								{menuBuiltInCommands.length > 0 && (
									<CommandGroup heading="Commands">
										{menuBuiltInCommands.map((command) => (
											<CommandItem
												key={command.id}
												value={`command:${command.id}`}
												className="items-start"
												onSelect={() => onCommandSelect(command)}
											>
												<div className="min-w-0 space-y-1">
													<div className="truncate font-mono text-content-primary text-xs">
														{command.trigger}
													</div>
													<div className="line-clamp-2 text-content-secondary text-xs leading-snug">
														{command.description}
													</div>
												</div>
											</CommandItem>
										))}
									</CommandGroup>
								)}
								{menuSkills.length > 0 && (
									<CommandGroup heading="Personal skills">
										{menuSkills.map((skill) => (
											<CommandItem
												key={skill.id}
												value={`skill:${skill.id}`}
												className="items-start"
												onSelect={() => onSelect(skill)}
											>
												<div className="min-w-0 space-y-1">
													<div className="truncate font-mono text-content-primary text-xs">
														{personalSkillTriggerText(skill)}
													</div>
													{skill.description.trim() && (
														<div className="line-clamp-2 text-content-secondary text-xs leading-snug">
															{skill.description}
														</div>
													)}
												</div>
											</CommandItem>
										))}
									</CommandGroup>
								)}
								{menuState?.isLoading && menuSkills.length === 0 && (
									<CommandItem value="loading" disabled>
										Loading personal skills...
									</CommandItem>
								)}
								{menuState?.isError && menuSkills.length === 0 && (
									<CommandItem value="error" disabled>
										Could not load personal skills. Close and type / again to
										retry.
									</CommandItem>
								)}
							</>
						)}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
