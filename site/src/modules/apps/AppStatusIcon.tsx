import type { WorkspaceAppStatus } from "api/typesGenerated";
import { Spinner } from "components/Spinner/Spinner";
import {
	CircleAlertIcon,
	CircleCheckIcon,
	HourglassIcon,
	TriangleAlertIcon,
} from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

type AppStatusIconProps = {
	status: WorkspaceAppStatus;
	latest: boolean;
	className?: string;
};

export const AppStatusIcon: FC<AppStatusIconProps> = ({
	status,
	latest,
	className: customClassName,
}) => {
	const className = cn(["size-4 shrink-0", customClassName]);

	switch (status.state) {
		case "complete":
			return (
				<CircleCheckIcon className={cn([className, "text-content-success"])} />
			);
		case "failure":
			return (
				<CircleAlertIcon className={cn([className, "text-content-warning"])} />
			);
		case "working":
			return latest ? (
				<Spinner size="sm" className="shrink-0" loading />
			) : (
				<HourglassIcon className={cn([className, "text-highlight-sky"])} />
			);
		default:
			return (
				<TriangleAlertIcon
					className={cn([className, "text-content-secondary"])}
				/>
			);
	}
};
