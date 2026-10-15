import { cn } from "cn";
import { useLayoutEffect, useRef } from "react";
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
import type { SkillTrigger } from "../../utils/skillAliases";

export type SkillMetadata = {
	name: string;
	description: string;
};

export type SkillMenuItem = Omit<SkillTrigger, "source"> & {
	source: SkillTrigger["source"] | "command";
};

// Built-in commands (e.g. /compact) share the menu item shape so the
// combined keyboard-selection list and trigger replacement work
// unchanged; their trigger text is never source-qualified.
export const createCommandMenuItem = (
	command: SkillMetadata,
): SkillMenuItem => ({
	name: command.name,
	description: command.description,
	source: "command",
	triggerText: `/${command.name}`,
	altTriggerText: `/${command.name}`,
});

type SkillsTriggerMenuProps = {
	open: boolean;
	// The composer box the menu is pinned above and sized to match.
	anchor: HTMLElement | null;
	query: string;
	commands: readonly SkillMenuItem[];
	personalSkills: readonly SkillMenuItem[];
	organizationSkills: readonly SkillMenuItem[];
	projectSkills: readonly SkillMenuItem[];
	workspaceSkills: readonly SkillMenuItem[];
	organizationSkillsEnabled: boolean;
	projectSkillsEnabled: boolean;
	workspaceSkillsEnabled: boolean;
	isPersonalLoading: boolean;
	isPersonalError: boolean;
	isOrganizationLoading: boolean;
	isOrganizationError: boolean;
	isProjectLoading: boolean;
	isProjectError: boolean;
	isWorkspaceLoading: boolean;
	selectedIndex: number;
	onSelectedIndexChange: (index: number) => void;
	onSelect: (skill: SkillMenuItem) => void;
	onClose: () => void;
	// Radix dismisses on Escape from a document capture-phase listener.
	// Call `event.preventDefault()` to keep the menu open so the editor's
	// own Escape handling can close it instead.
	onEscapeKeyDown: (event: KeyboardEvent) => void;
};

const getEmptyMessage = (
	query: string,
	organizationSkillsEnabled: boolean,
	projectSkillsEnabled: boolean,
	workspaceSkillsEnabled: boolean,
) => {
	if (query) {
		return organizationSkillsEnabled ||
			projectSkillsEnabled ||
			workspaceSkillsEnabled
			? "No skills match that query."
			: "No personal skills match that query.";
	}
	const sources = ["personal"];
	if (organizationSkillsEnabled) {
		sources.push("organization");
	}
	if (projectSkillsEnabled) {
		sources.push("project");
	}
	if (workspaceSkillsEnabled) {
		sources.push("workspace");
	}
	return `No ${new Intl.ListFormat("en-US", { type: "disjunction" }).format(sources)} skills found.`;
};

const SkillCommandItem = ({
	skill,
	value,
	selected,
	onSelect,
	consumePointerHighlight,
}: {
	skill: SkillMenuItem;
	value: string;
	selected: boolean;
	onSelect: (skill: SkillMenuItem) => void;
	consumePointerHighlight: () => boolean;
}) => {
	const handleSelect = () => onSelect(skill);
	const itemRef = useRef<HTMLDivElement>(null);

	// cmdk only auto-scrolls for its own key handling; arrow keys here are
	// consumed by the Lexical trigger plugin and arrive as a controlled value
	// change, so the item scrolls itself when it becomes the highlight.
	// Pointer highlights skip scrolling, like cmdk, to avoid hover/scroll loops.
	useLayoutEffect(() => {
		if (!selected || consumePointerHighlight()) {
			return;
		}
		const item = itemRef.current;
		if (!item) {
			return;
		}
		if (item.parentElement?.firstElementChild === item) {
			// First item in a group: reveal the group heading as well. cmdk
			// renders headings internally without exposing a ref, so locate
			// it through the DOM the same way cmdk does.
			item
				.closest("[cmdk-group]")
				?.querySelector("[cmdk-group-heading]")
				?.scrollIntoView({ block: "nearest" });
		}
		item.scrollIntoView({ block: "nearest" });
	}, [selected, consumePointerHighlight]);

	return (
		<CommandItem
			ref={itemRef}
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
	anchor,
	query,
	commands,
	personalSkills,
	organizationSkills,
	projectSkills,
	workspaceSkills,
	organizationSkillsEnabled,
	projectSkillsEnabled,
	workspaceSkillsEnabled,
	isPersonalLoading,
	isPersonalError,
	isOrganizationLoading,
	isOrganizationError,
	isProjectLoading,
	isProjectError,
	isWorkspaceLoading,
	selectedIndex,
	onSelectedIndexChange,
	onSelect,
	onClose,
	onEscapeKeyDown,
}: SkillsTriggerMenuProps) => {
	const pointerHighlightRef = useRef(false);

	// Groups follow selection order, so each starts where the previous ends.
	const skillGroups: {
		heading: string;
		skills: readonly SkillMenuItem[];
		startIndex: number;
	}[] = [];
	let nextStartIndex = commands.length;
	for (const [heading, skills] of [
		["Personal skills", personalSkills],
		["Organization skills", organizationSkills],
		["Project skills", projectSkills],
		["Workspace skills", workspaceSkills],
	] as const) {
		skillGroups.push({ heading, skills, startIndex: nextStartIndex });
		nextStartIndex += skills.length;
	}
	const itemCount = nextStartIndex;
	const statusItems = [
		isPersonalLoading && personalSkills.length === 0
			? "Loading personal skills..."
			: undefined,
		isPersonalError && personalSkills.length === 0
			? "Could not load personal skills. Close and type / again to retry."
			: undefined,
		isOrganizationLoading && organizationSkills.length === 0
			? "Loading organization skills..."
			: undefined,
		isOrganizationError && organizationSkills.length === 0
			? "Could not load organization skills. Close and type / again to retry."
			: undefined,
		isProjectLoading && projectSkills.length === 0
			? "Loading project skills..."
			: undefined,
		isProjectError && projectSkills.length === 0
			? "Could not load project skills. Close and type / again to retry."
			: undefined,
		isWorkspaceLoading && workspaceSkills.length === 0
			? "Loading workspace skills..."
			: undefined,
	].filter((item) => item !== undefined);
	const shouldRender = open && anchor !== null;
	const shouldShowEmpty = itemCount === 0 && statusItems.length === 0;
	const selectedValue = selectedIndex >= 0 ? String(selectedIndex) : "";

	const consumePointerHighlight = () => {
		const fromPointer = pointerHighlightRef.current;
		pointerHighlightRef.current = false;
		return fromPointer;
	};

	const handleHighlightedValueChange = (value: string) => {
		const nextIndex = Number(value);
		if (
			Number.isInteger(nextIndex) &&
			nextIndex >= 0 &&
			nextIndex < itemCount &&
			nextIndex !== selectedIndex
		) {
			pointerHighlightRef.current = true;
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
			consumePointerHighlight={consumePointerHighlight}
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
			{anchor && <PopoverAnchor virtualRef={{ current: anchor }} />}
			<PopoverContent
				align="start"
				side="top"
				sideOffset={8}
				className="w-(--radix-popper-anchor-width) overflow-hidden p-1 mobile-full-width-dropdown mobile-full-width-dropdown-above-composer"
				onMouseDown={(event) => event.preventDefault()}
				onOpenAutoFocus={(event) => event.preventDefault()}
				onCloseAutoFocus={(event) => event.preventDefault()}
				onEscapeKeyDown={onEscapeKeyDown}
			>
				<Command
					shouldFilter={false}
					loop={false}
					onValueChange={handleHighlightedValueChange}
					value={selectedValue}
				>
					<CommandList className="max-h-72 border-t-0 mobile-full-width-dropdown-scroll-area">
						{commands.length > 0 && (
							<CommandGroup heading="Commands">
								{commands.map((skill, index) => renderSkill(skill, index))}
							</CommandGroup>
						)}
						{skillGroups.map(({ heading, skills, startIndex }) =>
							skills.length > 0 ? (
								<CommandGroup key={heading} heading={heading}>
									{skills.map((skill, index) =>
										renderSkill(skill, startIndex + index),
									)}
								</CommandGroup>
							) : null,
						)}
						{statusItems.map((message) => (
							<CommandItem key={message} value={message} disabled>
								{message}
							</CommandItem>
						))}
						{shouldShowEmpty && (
							<CommandEmpty>
								{getEmptyMessage(
									query,
									organizationSkillsEnabled,
									projectSkillsEnabled,
									workspaceSkillsEnabled,
								)}
							</CommandEmpty>
						)}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
