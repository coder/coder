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

type SkillMetadata = {
	name: string;
	description: string;
};

export type SkillMenuItem = SkillMetadata & {
	source: SkillSource;
	triggerText: string;
	qualifiedTriggerText: string;
};

export const skillTriggerText = (skill: SkillMenuItem): string =>
	skill.triggerText;

export const createSkillMenuItem = (
	source: SkillSource,
	skill: SkillMetadata,
	useQualifiedAlias: boolean,
): SkillMenuItem => ({
	name: skill.name,
	description: skill.description,
	source,
	triggerText: `/${useQualifiedAlias ? `${source}/` : ""}${skill.name}`,
	qualifiedTriggerText: `/${source}/${skill.name}`,
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
	isWorkspaceError?: boolean;
	workspaceErrorMessage?: string;
	selectedIndex: number;
	onSelectedIndexChange: (index: number) => void;
	onSelect: (skill: SkillMenuItem) => void;
	onClose: () => void;
};

const skillValue = (skill: SkillMenuItem, index: number) =>
	`${skill.source}:${skill.name}:${index}`;

const selectedSkillValue = (
	personalSkills: readonly SkillMenuItem[],
	workspaceSkills: readonly SkillMenuItem[],
	selectedIndex: number,
): string => {
	if (selectedIndex < 0) {
		return "";
	}
	if (selectedIndex < personalSkills.length) {
		const skill = personalSkills[selectedIndex];
		return skill ? skillValue(skill, selectedIndex) : "";
	}
	const workspaceIndex = selectedIndex - personalSkills.length;
	const skill = workspaceSkills[workspaceIndex];
	return skill ? skillValue(skill, workspaceIndex) : "";
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
					{skillTriggerText(skill)}
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
	isWorkspaceError,
	workspaceErrorMessage,
	selectedIndex,
	onSelectedIndexChange,
	onSelect,
	onClose,
}: SkillsTriggerMenuProps) => {
	const handleHighlightedValueChange = (value: string) => {
		const personalIndex = personalSkills.findIndex(
			(skill, index) => skillValue(skill, index) === value,
		);
		if (personalIndex >= 0) {
			onSelectedIndexChange(personalIndex);
			return;
		}

		const workspaceIndex = workspaceSkills.findIndex(
			(skill, index) => skillValue(skill, index) === value,
		);
		if (workspaceIndex >= 0) {
			onSelectedIndexChange(personalSkills.length + workspaceIndex);
		}
	};

	const shouldRender = open && anchorRect;
	const hasPersonalSkills = personalSkills.length > 0;
	const hasWorkspaceSkills = workspaceSkills.length > 0;
	const showPersonalLoading = Boolean(isPersonalLoading && !hasPersonalSkills);
	const showWorkspaceLoading = Boolean(
		isWorkspaceLoading && !hasWorkspaceSkills,
	);
	const showPersonalError = Boolean(isPersonalError && !hasPersonalSkills);
	const showWorkspaceError = Boolean(isWorkspaceError && !hasWorkspaceSkills);
	const hasStatusItems =
		showPersonalLoading ||
		showWorkspaceLoading ||
		showPersonalError ||
		showWorkspaceError;
	const shouldShowEmpty =
		!hasPersonalSkills && !hasWorkspaceSkills && !hasStatusItems;
	const selectedValue = selectedSkillValue(
		personalSkills,
		workspaceSkills,
		selectedIndex,
	);
	const emptyMessage = getEmptyMessage(query, Boolean(workspaceSkillsEnabled));

	return (
		<Popover
			open={Boolean(shouldRender)}
			onOpenChange={(nextOpen) => {
				if (!nextOpen) {
					onClose();
				}
			}}
		>
			{shouldRender && (
				<PopoverAnchor asChild>
					<span
						aria-hidden="true"
						style={{
							position: "fixed",
							top: anchorRect.top,
							left: anchorRect.left,
							width: 1,
							height: Math.max(anchorRect.height, MIN_ANCHOR_HEIGHT_PX),
							pointerEvents: "none",
						}}
					/>
				</PopoverAnchor>
			)}
			<PopoverContent
				align="start"
				side="bottom"
				className="w-80 p-1"
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
					<CommandList className="max-h-72 border-t-0">
						{showPersonalLoading && (
							<CommandItem value="personal-loading" disabled>
								Loading personal skills...
							</CommandItem>
						)}
						{showPersonalError && (
							<CommandItem value="personal-error" disabled>
								Could not load personal skills. Close and type / again to retry.
							</CommandItem>
						)}
						{hasPersonalSkills && (
							<CommandGroup heading="Personal skills">
								{personalSkills.map((skill, index) => (
									<SkillCommandItem
										key={skillValue(skill, index)}
										skill={skill}
										value={skillValue(skill, index)}
										selected={index === selectedIndex}
										onSelect={onSelect}
									/>
								))}
							</CommandGroup>
						)}
						{hasWorkspaceSkills && (
							<CommandGroup heading="Workspace skills">
								{workspaceSkills.map((skill, index) => {
									const itemIndex = personalSkills.length + index;
									return (
										<SkillCommandItem
											key={skillValue(skill, index)}
											skill={skill}
											value={skillValue(skill, index)}
											selected={itemIndex === selectedIndex}
											onSelect={onSelect}
										/>
									);
								})}
							</CommandGroup>
						)}
						{showWorkspaceLoading && (
							<CommandItem value="workspace-loading" disabled>
								Loading workspace skills...
							</CommandItem>
						)}
						{showWorkspaceError && (
							<CommandItem value="workspace-error" disabled>
								{workspaceErrorMessage ??
									"Could not load workspace skills. Close and type / again to retry."}
							</CommandItem>
						)}
						{shouldShowEmpty && <CommandEmpty>{emptyMessage}</CommandEmpty>}
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
