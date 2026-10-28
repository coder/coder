import { EditIcon, EllipsisVerticalIcon, SettingsIcon } from "lucide-react";
import { Link } from "react-router";
import { foundationPath } from "#/api/queries/foundations";
import type { Foundation } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import { Stats, StatsItem } from "#/components/Stats/Stats";

type FoundationPageHeaderProps = {
	resources: Foundation;
	canUpdate: boolean;
};

/** Uses the template header primitives for Foundation identity and navigation. */
export const FoundationPageHeader: React.FC<FoundationPageHeaderProps> = ({
	resources,
	canUpdate,
}) => {
	const path = foundationPath(resources.organization_id, resources.id);
	return (
		<Margins>
			<PageHeader
				actions={
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button size="icon-lg" variant="subtle" aria-label="Open menu">
								<EllipsisVerticalIcon aria-hidden="true" />
								<span className="sr-only">Open menu</span>
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem asChild>
								<Link to={`${path}?tab=settings`}>
									<SettingsIcon className="size-icon-sm" />
									Settings
								</Link>
							</DropdownMenuItem>
							{canUpdate && (
								<DropdownMenuItem asChild>
									<Link to={`${path}?edit=true`}>
										<EditIcon />
										Edit files
									</Link>
								</DropdownMenuItem>
							)}
						</DropdownMenuContent>
					</DropdownMenu>
				}
			>
				<div className="flex flex-row gap-4">
					<Avatar size="lg" variant="icon" fallback={resources.name} />
					<div>
						<div className="flex flex-row items-center gap-2">
							<PageHeaderTitle>{resources.name}</PageHeaderTitle>
							<Badge
								variant={
									resources.status === "failed"
										? "destructive"
										: resources.status === "applied"
											? "green"
											: "outline"
								}
							>
								{resources.status}
							</Badge>
						</div>
						<PageHeaderSubtitle>
							Shared infrastructure managed independently of workspace builds.
						</PageHeaderSubtitle>
					</div>
				</div>
			</PageHeader>
			<div className="pb-8">
				<Stats>
					<StatsItem
						label="Current revision"
						value={
							resources.current_revision_id ? (
								<Link to={`${path}?tab=versions`}>
									{resources.current_revision_id.slice(0, 8)}
								</Link>
							) : (
								"None"
							)
						}
					/>
					<StatsItem
						label="Applied revision"
						value={
							resources.applied_revision_id ? (
								<Link to={`${path}?tab=versions`}>
									{resources.applied_revision_id.slice(0, 8)}
								</Link>
							) : (
								"None"
							)
						}
					/>
					<StatsItem label="Versions" value={resources.revisions.length} />
					<StatsItem label="Runs" value={resources.runs.length} />
				</Stats>
			</div>
		</Margins>
	);
};
