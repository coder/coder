import { Badge } from "#/components/Badge/Badge";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { DATE_FORMAT, formatDateTime } from "#/utils/time";

type ProjectMetadataBadgesProps = {
	/** The owner's display label. `undefined` while it loads. */
	readonly ownerLabel: string | undefined;
	readonly createdAt: string;
	/** Omit to hide the organization badge. */
	readonly organizationLabel?: string;
};

/** Owner, creation date, and organization of a chat project. */
export const ProjectMetadataBadges: React.FC<ProjectMetadataBadgesProps> = ({
	ownerLabel,
	createdAt,
	organizationLabel,
}) => (
	<ul className="m-0 flex list-none flex-wrap gap-2 p-0">
		<MetadataBadge label="Project owner">
			{ownerLabel ?? (
				<>
					<Skeleton className="h-2.5 w-12" />
					<span className="sr-only">Loading</span>
				</>
			)}
		</MetadataBadge>
		<MetadataBadge label="Created">
			<Tooltip>
				<TooltipTrigger asChild>
					<time dateTime={createdAt}>
						{formatDateTime(createdAt, DATE_FORMAT.MEDIUM_DATE)}
					</time>
				</TooltipTrigger>
				<TooltipContent>
					{formatDateTime(createdAt, DATE_FORMAT.FULL_DATETIME)}
				</TooltipContent>
			</Tooltip>
		</MetadataBadge>
		{organizationLabel !== undefined && (
			<MetadataBadge label="Organization">{organizationLabel}</MetadataBadge>
		)}
	</ul>
);

type MetadataBadgeProps = {
	readonly label: string;
	readonly children: React.ReactNode;
};

const MetadataBadge: React.FC<MetadataBadgeProps> = ({ label, children }) => (
	<li>
		<Badge size="sm">
			<span>{label}:</span>
			<span className="text-content-primary">{children}</span>
		</Badge>
	</li>
);
