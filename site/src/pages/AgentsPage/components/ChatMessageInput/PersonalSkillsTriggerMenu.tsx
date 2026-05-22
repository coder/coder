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
	const handleHighlightedValueChange = (value: string) => {
		const nextIndex = skills.findIndex((skill) => skill.name === value);
		if (nextIndex >= 0) {
			onSelectedIndexChange(nextIndex);
		}
	};

	const shouldRender = open && anchorRect;

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
				className="w-80 p-1 overflow-y-hidden md:overflow-y-auto mobile-full-width-dropdown mobile-full-width-dropdown-above-composer"
				onMouseDown={(event) => event.preventDefault()}
				onOpenAutoFocus={(event) => event.preventDefault()}
				onCloseAutoFocus={(event) => event.preventDefault()}
			>
				<Command
					shouldFilter={false}
					loop={false}
					onValueChange={handleHighlightedValueChange}
					value={skills[selectedIndex]?.name ?? ""}
				>
					<CommandList className="max-h-[calc(var(--mobile-dropdown-above-composer-max-height,18.5rem)-0.5rem)] border-t-0 md:max-h-72">
						{isLoading ? (
							<CommandItem value="loading" disabled>
								Loading personal skills...
							</CommandItem>
						) : isError ? (
							<CommandItem value="error" disabled>
								Could not load personal skills. Close and type / again to retry.
							</CommandItem>
						) : skills.length === 0 ? (
							<CommandEmpty>
								{query
									? "No personal skills match that query."
									: "No personal skills found."}
							</CommandEmpty>
						) : (
							<CommandGroup heading="Personal skills">
								{skills.map((skill) => (
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
