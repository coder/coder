import { CheckIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";

const sections = [
	{ label: "Workspaces", to: "/workspaces" },
	{ label: "Templates", to: "/templates" },
	{ label: "Agents", to: "/agents" },
] as const;

const currentSection = "/agents";

/**
 * Switches between the top-level dashboard sections from the agents
 * sidebar header. The sidebar only renders under /agents, so that
 * section is always the checked one.
 */
export const SectionSwitcher: FC = () => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="sm"
					className="group h-7 min-w-0 gap-1 px-1.5 text-sm text-content-primary"
				>
					Agents
					<ChevronDownIcon className="text-content-secondary" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start">
				{sections.map((section) => {
					const isCurrent = section.to === currentSection;
					return (
						<DropdownMenuItem
							key={section.to}
							asChild
							className={isCurrent ? "text-content-primary" : undefined}
						>
							<Link
								to={section.to}
								aria-current={isCurrent ? "page" : undefined}
							>
								{section.label}
								{isCurrent && <CheckIcon className="ml-auto" />}
							</Link>
						</DropdownMenuItem>
					);
				})}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
