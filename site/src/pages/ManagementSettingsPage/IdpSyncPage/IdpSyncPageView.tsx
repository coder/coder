import type { Interpolation, Theme } from "@emotion/react";
import LaunchOutlined from "@mui/icons-material/LaunchOutlined";
import Button from "@mui/material/Button";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import type { Role } from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Paywall } from "components/Paywall/Paywall";
import { Stack } from "components/Stack/Stack";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import type { FC } from "react";
import { docs } from "utils/docs";

export type IdpSyncPageViewProps = {
	roles: Role[] | undefined;
};

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({ roles }) => {
	return (
		<>
			<ChooseOne>
				<Cond condition={false}>
					<Paywall
						message="IdP Sync"
						description="Configure group and role mappings to manage permissions outside of Coder."
						documentationLink={docs("/admin/groups")}
					/>
				</Cond>
				<Cond>
					<Stack spacing={2} css={styles.fields}>
						{/* Semantically fieldset is used for forms. In the future this screen will allow
						 updates to these fields in a form */}
						<fieldset css={styles.box}>
							<legend css={styles.legend}>Groups</legend>
							<Stack direction={"row"} alignItems={"center"} spacing={3}>
								<h4>Sync Field</h4>
								<p css={styles.secondary}>groups</p>
								<h4>Regex Filter</h4>
								<p css={styles.secondary}>^Coder-.*$</p>
								<h4>Auto Create</h4>
								<p css={styles.secondary}>false</p>
							</Stack>
						</fieldset>
						<fieldset css={styles.box}>
							<legend css={styles.legend}>Roles</legend>
							<Stack direction={"row"} alignItems={"center"} spacing={3}>
								<h4>Sync Field</h4>
								<p css={styles.secondary}>roles</p>
							</Stack>
						</fieldset>
					</Stack>
					<Stack spacing={4}>
						<RoleTable roles={roles} />
						<RoleTable roles={roles} />
					</Stack>
				</Cond>
			</ChooseOne>
		</>
	);
};

interface RoleTableProps {
	roles: Role[] | undefined;
}

const RoleTable: FC<RoleTableProps> = ({ roles }) => {
	const isLoading = false;
	const isEmpty = Boolean(roles && roles.length === 0);
	return (
		<TableContainer>
			<Table>
				<TableHead>
					<TableRow>
						<TableCell width="45%">Idp Role</TableCell>
						<TableCell width="55%">Coder Role</TableCell>
					</TableRow>
				</TableHead>
				<TableBody>
					<ChooseOne>
						<Cond condition={isLoading}>
							<TableLoader />
						</Cond>

						<Cond condition={isEmpty}>
							<TableRow>
								<TableCell colSpan={999}>
									<EmptyState
										message="No Role Mappings"
										description={
											"Configure role sync mappings to manage permissions outside of Coder."
										}
										isCompact
										cta={
											<Button
												startIcon={<LaunchOutlined />}
												component="a"
												href={docs("/admin/auth#group-sync-enterprise")}
												target="_blank"
											>
												How to setup IdP role sync
											</Button>
										}
									/>
								</TableCell>
							</TableRow>
						</Cond>

						<Cond>
							{roles?.map((role) => (
								<RoleRow key={role.name} role={role} />
							))}
						</Cond>
					</ChooseOne>
				</TableBody>
			</Table>
		</TableContainer>
	);
};

interface RoleRowProps {
	role: Role;
}

const RoleRow: FC<RoleRowProps> = ({ role }) => {
	return (
		<TableRow data-testid={`role-${role.name}`}>
			<TableCell>{role.display_name || role.name}</TableCell>
			<TableCell css={styles.secondary}>test</TableCell>
		</TableRow>
	);
};

const TableLoader = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
				<TableCell>
					<Skeleton variant="text" width="25%" />
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

const styles = {
	secondary: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	fields: (theme) => ({
		marginBottom: "60px",
	}),
	legend: (theme) => ({
		padding: "0px 6px",
		fontWeight: 600,
	}),
	box: (theme) => ({
		border: "1px solid",
		borderColor: theme.palette.divider,
		padding: "0px 20px",
		borderRadius: 8,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default IdpSyncPageView;
