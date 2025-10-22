import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
import {
	Table,
	TableBody,
	TableCell,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import type { FC } from "react";
import { docs } from "utils/docs";

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
					<ChooseOne>
						<Cond condition={rowCount === 0}>
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message={`No ${type.toLocaleLowerCase()} mappings`}
										isCompact
										cta={
											<Link
												href={docs(
													`/admin/users/idp-sync#${type.toLocaleLowerCase()}-sync`,
												)}
											>
												How to setup IdP {type.toLocaleLowerCase()} sync
											</Link>
										}
									/>
								</TableCell>
							</TableRow>
						</Cond>
						<Cond>{children}</Cond>
					</ChooseOne>
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
