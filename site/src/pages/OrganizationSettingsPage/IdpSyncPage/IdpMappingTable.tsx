import {
	Table,
	TableBody,
	TableCell,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmptyState } from "#/components/TableEmptyState/TableEmptyState";
import { docs } from "#/utils/docs";

type IdpMappingTableProps = {
	type: "Role" | "Group";
	rowCount: number;
	children: React.ReactNode;
};

export const IdpMappingTable: React.FC<IdpMappingTableProps> = ({
	type,
	rowCount,
	children,
}) => {
	return (
		<div className="flex flex-col gap-2">
			<Table>
				<TableHeader>
					<TableRow>
						<TableCell className="w-2/5">
							IdP {type.toLocaleLowerCase()}
						</TableCell>
						<TableCell className="w-3/5">
							Coder {type.toLocaleLowerCase()}
						</TableCell>
						<TableCell className="w-auto" />
					</TableRow>
				</TableHeader>
				<TableBody>
					{rowCount === 0 ? (
						<TableEmptyState
							variant="docs"
							message={`No ${type.toLocaleLowerCase()} mappings configured`}
							description={
								type === "Group"
									? "Automatically assign users to groups based on their identity provider claims."
									: "Automatically assign roles to users based on their identity provider claims."
							}
							docsHref={docs(
								`/admin/users/idp-sync#${type.toLocaleLowerCase()}-sync`,
							)}
							docsLabel={`How to set up IdP ${type.toLocaleLowerCase()} sync`}
						/>
					) : (
						children
					)}
				</TableBody>
			</Table>
			<div className="flex justify-end">
				<div className="text-content-secondary text-xs">
					Showing <strong className="text-content-primary">{rowCount}</strong>{" "}
					{type.toLocaleLowerCase()}
					{(rowCount === 0 || rowCount > 1) && "s"}
				</div>
			</div>
		</div>
	);
};
