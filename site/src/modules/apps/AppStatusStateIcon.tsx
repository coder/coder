import type { WorkspaceAppStatusState } from "api/typesGenerated";
import { Spinner } from "components/Spinner/Spinner";
import {
	BanIcon,
	CircleAlertIcon,
	CircleCheckIcon,
	HourglassIcon,
	TriangleAlertIcon,
} from "lucide-react";
import type { FC } from "react";
import { cn } from "utils/cn";

type AppStatusStateIconProps = {
	state: WorkspaceAppStatusState;
	latest: boolean;
	disabled?: boolean;
	className?: string;
};

export const AppStatusStateIcon: FC<AppStatusStateIconProps> = ({
	state,
	disabled,
	latest,
	className: customClassName,
}) => {
	const className = cn(["size-4 shrink-0", customClassName]);

	switch (state) {
		case "complete":
			return (
				<CircleCheckIcon className={cn(["text-content-success", className])} />
			);
		case "failure":
			return (
				<CircleAlertIcon className={cn(["text-content-warning", className])} />
			);
		case "working":
			return disabled ? (
				<BanIcon className={cn(["text-content-disabled", className])} />
			) : latest ? (
				<Spinner size="sm" className="shrink-0" loading />
			) : (
				<HourglassIcon className={cn(["text-content-secondary", className])} />
			);
		default:
			return (
				<TriangleAlertIcon
					className={cn(["text-content-secondary", className])}
				/>
			);
	}
};
