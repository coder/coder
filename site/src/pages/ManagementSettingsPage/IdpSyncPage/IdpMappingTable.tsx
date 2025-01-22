import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Link } from "components/Link/Link";
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
		<div className="flex flex-col w-full gap-2">
			<TableContainer>
				<Table>
					<TableHead>
						<TableRow>
							<TableCell width="45%">IdP {type.toLocaleLowerCase()}</TableCell>
							<TableCell width="55%">
								Coder {type.toLocaleLowerCase()}
							</TableCell>
							<TableCell width="10%" />
						</TableRow>
					</TableHead>
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
			</TableContainer>
			<div className="flex justify-end">
				<div className="text-content-secondary text-xs">
					Showing <strong className="text-content-primary">{rowCount}</strong>{" "}
					groups
				</div>
			</div>
		</div>
	);
};
