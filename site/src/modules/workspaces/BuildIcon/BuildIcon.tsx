import {
	type LucideProps,
	PlayIcon,
	SquareIcon,
	TrashIcon,
} from "lucide-react";
import type {
	ProvisionerJobStatus,
	WorkspaceTransition,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { cn } from "#/utils/cn";

type BuildIconProps = LucideProps & {
	transition: WorkspaceTransition;
	jobStatus: ProvisionerJobStatus;
	avatar?: boolean;
};

const iconByTransition: Record<
	WorkspaceTransition,
	React.ComponentType<LucideProps>
> = {
	start: PlayIcon,
	stop: SquareIcon,
	delete: TrashIcon,
};

const statusColors: Record<ProvisionerJobStatus, string> = {
	succeeded: "text-content-success",
	pending: "text-content-success",
	running: "text-content-success",
	failed: "text-content-success",
	canceling: "text-content-success",
	canceled: "text-content-success",
	unknown: "text-content-success",
};

export const BuildIcon: React.FC<BuildIconProps> = ({
	transition,
	jobStatus,
	avatar,
	className,
	...props
}) => {
	const Icon = iconByTransition[transition];

	return avatar ? (
		<Avatar size="lg" variant="icon">
			<Icon className={cn("size-full", statusColors[jobStatus], className)} />
		</Avatar>
	) : (
		<Icon
			className={cn("size-4", statusColors[jobStatus], className)}
			{...props}
		/>
	);
};
