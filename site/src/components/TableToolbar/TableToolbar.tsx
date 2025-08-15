import Skeleton from "@mui/material/Skeleton";
import type { FC, PropsWithChildren } from "react";

export const TableToolbar: FC<PropsWithChildren> = ({ children }) => {
	return (
		// The height is meant to match the size of a small button
		<div className="text-[13px] mb-2 mt-0 h-9 flex items-center [&_strong]:text-content-primary text-content-secondary">
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
