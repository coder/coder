import { UsersIcon } from "lucide-react";
import type { SharedWorkspaceActor } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Link } from "#/components/Link/Link";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";

type WorkspaceSharingIndicatorProps = {
	sharedWith: readonly SharedWorkspaceActor[];
	settingsPath: string;
};

export const WorkspaceSharingIndicator: React.FC<
	WorkspaceSharingIndicatorProps
> = ({ sharedWith, settingsPath }) => {
	// Sort by type (users then groups) and then alphabetically by name.
	const sortedActors = [...sharedWith].sort((a, b) => {
		if (a.actor_type !== b.actor_type) {
			return a.actor_type === "user" ? -1 : 1;
		}
		return a.name.localeCompare(b.name);
	});

	return (
		<Tooltip interactive>
			<TooltipTrigger
				aria-label="Workspace permissions"
				// The 24px target (WCAG 2.5.8) extends only downward so it does not
				// cover the workspace name link above, and the negative margins keep
				// the row layout unchanged.
				className="size-6 -mx-1.25 -mb-2 pb-2 justify-center text-content-secondary hover:text-content-primary"
			>
				<UsersIcon className="size-icon-xs" />
			</TooltipTrigger>
			<TooltipContent className="w-56 p-0">
				<div className="px-3 py-2">
					<h2 className="m-0 text-sm font-semibold text-content-primary">
						Workspace permissions
					</h2>
				</div>
				<ul className="flex flex-col gap-1 m-0 p-0 list-none max-h-48 overflow-y-auto">
					{sortedActors.map((actor) => {
						const isAdmin = actor.roles.includes("admin");
						return (
							<li
								key={actor.id}
								className="flex items-center px-3 gap-2 text-sm text-content-secondary"
							>
								<span className="text-sm truncate">{actor.name}</span>
								{isAdmin && (
									<Badge size="sm" variant="default">
										Admin
									</Badge>
								)}
							</li>
						);
					})}
				</ul>
				<div className="px-3 pb-3 pt-4">
					<Link
						href={settingsPath}
						className="text-sm text-content-link font-medium"
						onClick={(e) => e.stopPropagation()}
						showExternalIcon={false}
					>
						Change permissions
					</Link>
				</div>
			</TooltipContent>
		</Tooltip>
	);
};
