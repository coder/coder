import { cn } from "cn";
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react";
import type { FC } from "react";
import { Link } from "react-router";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";

const AREAS = [
	{ label: "Workspaces", to: "/workspaces" },
	{ label: "Templates", to: "/templates" },
	{ label: "Agents", to: "/agents" },
] as const;

const CURRENT_AREA = "Agents";

/**
 * Names the area the sidebar belongs to and lets the user jump to the other
 * top-level dashboard areas without leaving the sidebar header.
 */
export const SidebarAreaMenu: FC = () => {
	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button
					variant="subtle"
					size="sm"
					className="h-7 min-w-0 gap-1 px-1.5 text-sm font-medium text-content-primary [&>svg]:size-icon-xs [&>svg]:p-0"
				>
					{CURRENT_AREA}
					<ChevronsUpDownIcon className="text-content-secondary" />
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="min-w-40">
				{AREAS.map((area) => {
					const isCurrent = area.label === CURRENT_AREA;
					return (
						<DropdownMenuItem
							key={area.to}
							asChild
							className={cn(isCurrent && "text-content-primary")}
						>
							<Link to={area.to} aria-current={isCurrent ? "page" : undefined}>
								{area.label}
								{isCurrent && <CheckIcon className="ml-auto" />}
							</Link>
						</DropdownMenuItem>
					);
				})}
			</DropdownMenuContent>
		</DropdownMenu>
	);
};
