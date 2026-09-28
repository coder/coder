import { cloneElement, isValidElement } from "react";
import {
	TableCell,
	TableRow,
	type TableRowProps,
} from "#/components/Table/Table";
import { Loader } from "../Loader/Loader";

export const TableLoader: React.FC = () => {
	return (
		<TableRow>
			<TableCell colSpan={999} className="text-center h-40">
				<Loader />
			</TableCell>
		</TableRow>
	);
};

type TableLoaderSkeletonProps = {
	rows?: number;
	children?: React.ReactNode;
};

export const TableLoaderSkeleton: React.FC<TableLoaderSkeletonProps> = ({
	rows = 4,
	children,
}) => {
	if (!isValidElement(children)) {
		throw new Error(
			"TableLoaderSkeleton children must be a valid React element",
		);
	}
	return (
		<>
			{Array.from({ length: rows }, (_, i) =>
				cloneElement(children, { key: i }),
			)}
		</>
	);
};

export const TableRowSkeleton: React.FC<TableRowProps> = ({
	children,
	...rowProps
}) => {
	return (
		<TableRow role="progressbar" data-testid="loader" {...rowProps}>
			{children}
		</TableRow>
	);
};
