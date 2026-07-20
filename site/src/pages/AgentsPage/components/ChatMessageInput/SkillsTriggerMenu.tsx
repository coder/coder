import { useState } from "react";
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
import { cn } from "#/utils/cn";

// Prevent zero-height anchors when the browser returns a degenerate caret rect.
const MIN_ANCHOR_HEIGHT_PX = 16;

export type CaretAnchorRect = {
	top: number;
	left: number;
	height: number;
};

type SkillSource = "personal" | "workspace";

export type SkillMetadata = {
	name: string;
	description: string;
};

export type SkillMenuItem = SkillMetadata & {
	source: SkillSource;
	triggerText: string;
	// The qualified alias stays searchable even when the displayed
	// trigger is bare, so a typed qualified query keeps matching after
	// collision state changes mid-trigger.
	altTriggerText: string;
};

export const createSkillMenuItem = (
	source: SkillSource,
	skill: SkillMetadata,
	// Bare personal names are ambiguous to read_skill when a workspace
	// skill shares the name, so colliding triggers must stay qualified.
	qualifyTrigger = source === "workspace",
): SkillMenuItem => ({
	name: skill.name,
	description: skill.description,
	source,
	triggerText: qualifyTrigger ? `/${source}/${skill.name}` : `/${skill.name}`,
	altTriggerText: `/${source}/${skill.name}`,
});

type SkillsTriggerMenuProps = {
	open: boolean;
	anchorRect: CaretAnchorRect | null;
	query: string;
	personalSkills: readonly SkillMenuItem[];
	workspaceSkills: readonly SkillMenuItem[];
	workspaceSkillsEnabled?: boolean;
	isPersonalLoading?: boolean;
	isPersonalError?: boolean;
	isWorkspaceLoading?: boolean;
	selectedIndex: number;
	onSelectedIndexChange: (index: number) => void;
	onSelect: (skill: SkillMenuItem) => void;
	onClose: () => void;
};

const getEmptyMessage = (query: string, workspaceSkillsEnabled: boolean) => {
	if (query) {
		return workspaceSkillsEnabled
			? "No skills match that query."
			: "No personal skills match that query.";
	}
	return workspaceSkillsEnabled
		? "No personal or workspace skills found."
		: "No personal skills found.";
};

const SkillCommandItem = ({
	skill,
	value,
	selected,
	onSelect,
}: {
	skill: SkillMenuItem;
	value: string;
	selected: boolean;
	onSelect: (skill: SkillMenuItem) => void;
}) => {
	const handleSelect = () => onSelect(skill);

	return (
		<CommandItem
			value={value}
			aria-selected={selected}
			className={cn(
				"items-start",
				selected && "bg-surface-secondary text-content-primary",
			)}
			onSelect={handleSelect}
		>
			<div className="min-w-0 space-y-1">
				<div className="truncate font-mono text-content-primary text-xs">
					{skill.triggerText}
				</div>
				{skill.description.trim() && (
					<div className="line-clamp-2 text-content-secondary text-xs leading-snug">
						{skill.description}
					</div>
				)}
			</div>
		</CommandItem>
	);
};

export const SkillsTriggerMenu = ({
	open,
	anchorRect,
	query,
	personalSkills,
	workspaceSkills,
	workspaceSkillsEnabled,
	isPersonalLoading,
	isPersonalError,
	isWorkspaceLoading,
	selectedIndex,
	onSelectedIndexChange,
	onSelect,
	onClose,
}: SkillsTriggerMenuProps) => {
	const allSkills = [...personalSkills, ...workspaceSkills];
	const statusItems = [
		isPersonalLoading && personalSkills.length === 0
			? "Loading personal skills..."
			: undefined,
		isPersonalError && personalSkills.length === 0
			? "Could not load personal skills. Close and type / again to retry."
			: undefined,
		isWorkspaceLoading && workspaceSkills.length === 0
			? "Loading workspace skills..."
			: undefined,
	].filter((item) => item !== undefined);
	const shouldRender = open && anchorRect;
	// Radix keeps closing content mounted through its exit animation.
	// Unmounting the anchor then would reposition the closing menu to
	// the viewport origin, so keep it at the last known caret rect.
	const [lastAnchorRect, setLastAnchorRect] = useState(anchorRect);
	if (anchorRect && anchorRect !== lastAnchorRect) {
		setLastAnchorRect(anchorRect);
	}
	const renderedAnchorRect = anchorRect ?? lastAnchorRect;
	const shouldShowEmpty = allSkills.length === 0 && statusItems.length === 0;
	const selectedValue = selectedIndex >= 0 ? String(selectedIndex) : "";

	const handleHighlightedValueChange = (value: string) => {
		const nextIndex = Number(value);
		if (
			Number.isInteger(nextIndex) &&
			nextIndex >= 0 &&
			nextIndex < allSkills.length
		) {
			onSelectedIndexChange(nextIndex);
		}
	};

	const renderSkill = (skill: SkillMenuItem, index: number) => (
		<SkillCommandItem
			key={`${skill.source}:${skill.name}:${index}`}
			skill={skill}
			value={String(index)}
			selected={index === selectedIndex}
			onSelect={onSelect}
		/>
	);

	return (
		<Popover
			open={Boolean(shouldRender)}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onClose();
				}
			}}
		>
			{renderedAnchorRect && (
				<PopoverAnchor asChild>
					<span
						aria-hidden="true"
						style={{
							position: "fixed",
							top: renderedAnchorRect.top,
							left: renderedAnchorRect.left,
							width: 1,
							height: Math.max(renderedAnchorRect.height, MIN_ANCHOR_HEIGHT_PX),
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
						{personalSkills.length > 0 && (
							<CommandGroup heading="Personal skills">
								{personalSkills.map((skill, index) =>
									renderSkill(skill, index),
								)}
							</CommandGroup>
						)}
						{workspaceSkills.length > 0 && (
							<CommandGroup heading="Workspace skills">
								{workspaceSkills.map((skill, index) =>
									renderSkill(skill, personalSkills.length + index),
								)}
							</CommandGroup>
						)}
						{statusItems.map((message) => (
							<CommandItem key={message} value={message} disabled>
								{message}
							</CommandItem>
						))}
						{shouldShowEmpty && (
							<CommandEmpty>
								{getEmptyMessage(query, Boolean(workspaceSkillsEnabled))}
							</CommandEmpty>
						)}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
