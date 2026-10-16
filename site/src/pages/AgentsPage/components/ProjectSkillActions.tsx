import { ChevronDownIcon } from "lucide-react";
import { useRef } from "react";
import { useQuery } from "react-query";
import { chatProjectSkills } from "#/api/queries/chatProjectSkills";
import type { SkillMetadata } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

const MAX_INLINE_ACTIONS = 3;

type ProjectSkillActionsProps = {
	organizationId: string;
	projectId: string;
	onInvoke: (name: string) => void;
};

/**
 * Offers a project's action skills above a composer. It renders nothing
 * until the skills load, so the composer never waits on them.
 */
export const ProjectSkillActions: React.FC<ProjectSkillActionsProps> = ({
	organizationId,
	projectId,
	onInvoke,
}) => {
	// Invoking from the menu waits for it to close, because the menu
	// traps focus and then returns it to its trigger, away from the composer.
	const menuSelectionRef = useRef<string | null>(null);
	// The app skips focus refetches, but another tab may change the actions.
	const skillsQuery = useQuery({
		...chatProjectSkills(organizationId, projectId),
		refetchOnWindowFocus: true,
	});
	const actions = (skillsQuery.data ?? [])
		.filter((skill) => skill.enabled && skill.action)
		.toSorted((a, b) => a.name.localeCompare(b.name, "en-US"));
	if (actions.length === 0) {
		return null;
	}
	const inlineActions = actions.slice(0, MAX_INLINE_ACTIONS);
	const menuActions = actions.slice(MAX_INLINE_ACTIONS);

	return (
		<div
			role="group"
			aria-label="Project actions"
			className="mb-2 flex flex-wrap items-center gap-1.5"
		>
			{inlineActions.map((skill) => (
				<ProjectSkillActionButton
					key={skill.id}
					skill={skill}
					onInvoke={onInvoke}
				/>
			))}
			{menuActions.length > 0 && (
				<DropdownMenu>
					<DropdownMenuTrigger asChild>
						<Button
							size="sm"
							variant="outline"
							aria-label="More project actions"
						>
							More
							<ChevronDownIcon aria-hidden="true" />
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent
						align="start"
						className="max-w-80"
						onCloseAutoFocus={(event) => {
							const name = menuSelectionRef.current;
							menuSelectionRef.current = null;
							if (name !== null) {
								event.preventDefault();
								onInvoke(name);
							}
						}}
					>
						{menuActions.map((skill) => (
							<DropdownMenuItem
								key={skill.id}
								className="flex-col items-start gap-0.5"
								onSelect={() => {
									menuSelectionRef.current = skill.name;
								}}
							>
								<span className="max-w-full truncate text-content-primary">
									{skill.name}
								</span>
								{skill.description && (
									<span className="line-clamp-2 text-xs font-normal">
										{skill.description}
									</span>
								)}
							</DropdownMenuItem>
						))}
					</DropdownMenuContent>
				</DropdownMenu>
			)}
		</div>
	);
};

type ProjectSkillActionButtonProps = {
	skill: SkillMetadata;
	onInvoke: (name: string) => void;
};

const ProjectSkillActionButton: React.FC<ProjectSkillActionButtonProps> = ({
	skill,
	onInvoke,
}) => (
	// The tooltip shows the full name, which a narrow composer truncates.
	<Tooltip>
		<TooltipTrigger asChild>
			<Button
				size="sm"
				variant="outline"
				className="max-w-full"
				onClick={() => onInvoke(skill.name)}
			>
				<span className="truncate">{skill.name}</span>
			</Button>
		</TooltipTrigger>
		<TooltipContent className="max-w-64">
			<span className="block break-words font-medium">{skill.name}</span>
			{skill.description && <span className="block">{skill.description}</span>}
		</TooltipContent>
	</Tooltip>
);
