import Skeleton from "@mui/material/Skeleton";
import type { FC, PropsWithChildren } from "react";
import { cn } from "utils/cn";

export type TableToolbarProps = Readonly<
	PropsWithChildren<{
		className?: string;
	}>
>;

export const TableToolbar: FC<TableToolbarProps> = ({
	className,
	children,
}) => {
	return (
		<div
			className={cn(
				"text-sm mb-2 mt-0 h-9 text-content-secondary flex items-center [&_strong]:text-content-primary",
				className,
			)}
		>
			{children}
		</div>
	);
};

type PaginationStatusProps =
	| BasePaginationStatusProps
	| LoadedPaginationStatusProps;

type BasePaginationStatusProps = {
	isLoading: true;
};

type LoadedPaginationStatusProps = {
	isLoading: false;
	label: string;
	showing: number;
	total: number;
};

export const PaginationStatus: FC<PaginationStatusProps> = (props) => {
	const { isLoading } = props;

	if (isLoading) {
		return (
			<div className="h-6 flex items-center">
				<Skeleton variant="text" width={160} height={16} />
			</div>
		);
	}

	const { showing, total, label } = props;

	return (
		<div>
			Showing <strong>{showing}</strong> of{" "}
			<strong>{total?.toLocaleString()}</strong> {label}
		</div>
	);
};
