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
	skills: readonly TypesGen.UserSkillMetadata[];
	isLoading?: boolean;
	isError?: boolean;
	selectedIndex: number;
	onSelectedIndexChange: (index: number) => void;
	onSelect: (skill: TypesGen.UserSkillMetadata) => void;
	onClose: () => void;
};

type PersonalSkillsMenuState = {
	anchorRect: CaretAnchorRect;
	query: string;
	skills: readonly TypesGen.UserSkillMetadata[];
	isLoading?: boolean;
	isError?: boolean;
	selectedIndex: number;
};

export const PersonalSkillsTriggerMenu = ({
	open,
	anchorRect,
	query,
	skills,
	isLoading,
	isError,
	selectedIndex,
	onSelectedIndexChange,
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
				skills,
				isLoading,
				isError,
				selectedIndex,
			}
		: null;
	const menuState = activeMenuState ?? lastOpenMenuState;
	const menuAnchorRect = menuState?.anchorRect ?? null;
	const menuSkills = menuState?.skills ?? [];
	const menuSelectedIndex = menuState?.selectedIndex ?? -1;

	useLayoutEffect(() => {
		if (!isAnchoredOpen) {
			return;
		}
		setLastOpenMenuState({
			anchorRect,
			query,
			skills,
			isLoading,
			isError,
			selectedIndex,
		});
	}, [
		anchorRect,
		isAnchoredOpen,
		isError,
		isLoading,
		query,
		selectedIndex,
		skills,
	]);

	const handleHighlightedValueChange = (value: string) => {
		const nextIndex = menuSkills.findIndex((skill) => skill.name === value);
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
					value={menuSkills[menuSelectedIndex]?.name ?? ""}
				>
					<CommandList className="max-h-72 border-t-0 mobile-full-width-dropdown-scroll-area">
						{menuState?.isLoading ? (
							<CommandItem value="loading" disabled>
								Loading personal skills...
							</CommandItem>
						) : menuState?.isError ? (
							<CommandItem value="error" disabled>
								Could not load personal skills. Close and type / again to retry.
							</CommandItem>
						) : menuSkills.length === 0 ? (
							<CommandEmpty>
								{menuState?.query
									? "No personal skills match that query."
									: "No personal skills found."}
							</CommandEmpty>
						) : (
							<CommandGroup heading="Personal skills">
								{menuSkills.map((skill) => (
									<CommandItem
										key={skill.id}
										value={skill.name}
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
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
