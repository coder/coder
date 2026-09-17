import type { FC } from "react";
import {
	Table,
	TableBody,
	TableCell,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { IdpSyncEmptyState } from "#/modules/idpSync/IdpSyncEmptyState";
import { docs } from "#/utils/docs";

interface IdpMappingTableProps {
	type: "Role" | "Group";
	rowCount: number;
	children: React.ReactNode;
}

export const IdpMappingTable: FC<IdpMappingTableProps> = ({
	type,
	rowCount,
	children,
}) => {
	const label = type.toLocaleLowerCase();

	if (rowCount === 0) {
		return (
			<IdpSyncEmptyState
				title={`No IdP ${label} sync configured`}
				description={
					type === "Group"
						? "Automatically assign users to groups based on their identity provider claims."
						: "Automatically assign roles to users based on their identity provider claims."
				}
				ctaLabel={`Set up IdP ${label} sync`}
				docsHref={docs(`/admin/users/idp-sync#${label}-sync`)}
			/>
		);
	}

	return (
		<div className="flex flex-col gap-2">
			<Table>
				<TableHeader>
					<TableRow>
						<TableCell className="w-2/5">IdP {label}</TableCell>
						<TableCell className="w-3/5">Coder {label}</TableCell>
						<TableCell className="w-auto" />
					</TableRow>
				</TableHeader>
				<TableBody>{children}</TableBody>
			</Table>
			<div className="flex justify-end">
				<div className="text-content-secondary text-xs">
					Showing <strong className="text-content-primary">{rowCount}</strong>{" "}
					{label}
					{rowCount > 1 && "s"}
				</div>
			</div>
		</div>
	);
};
