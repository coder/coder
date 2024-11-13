import type { Interpolation, Theme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import TextField from "@mui/material/TextField";
import type {
	Organization,
	OrganizationSyncSettings,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import { Stack } from "components/Stack/Stack";
import { StatusIndicator } from "components/StatusIndicator/StatusIndicator";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import { Button } from "components/ui/button";
import { Label } from "components/ui/label";
import MultipleSelector, { type Option } from "components/ui/multiple-selector";
import { Switch } from "components/ui/switch";
// import { Input } from "components/ui/input";
import { useFormik } from "formik";
import { Plus, SquareArrowOutUpRight } from "lucide-react";
import type React from "react";
import { useState } from "react";
import type { FC } from "react";
import { MONOSPACE_FONT_FAMILY } from "theme/constants";
import { docs } from "utils/docs";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
// import { ExportPolicyButton } from "./ExportPolicyButton";
import { IdpPillList } from "./IdpPillList";

interface IdpSyncPageViewProps {
	organizationSyncSettings: OrganizationSyncSettings | undefined;
	organizations: readonly Organization[];
	onSubmit: (data: OrganizationSyncSettings) => void;
	error?: unknown;
}

export const IdpSyncPageView: FC<IdpSyncPageViewProps> = ({
	organizationSyncSettings,
	organizations,
	onSubmit,
	error,
}) => {
	const form = useFormik<OrganizationSyncSettings>({
		initialValues: {
			field: organizationSyncSettings?.field || "",
			organization_assign_default:
				organizationSyncSettings?.organization_assign_default || true,
			mapping: organizationSyncSettings?.mapping || {},
		},
		// validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers<OrganizationSyncSettings>(form, error);
	const [coderOrgs, setCoderOrgs] = useState<Option[]>([]);
	const [isChecked, setIsChecked] = useState(
		form.initialValues.organization_assign_default,
	);
	const organizationMappingCount = organizationSyncSettings?.mapping
		? Object.entries(organizationSyncSettings.mapping).length
		: 0;

	if (error) {
		return <ErrorAlert error={error} />;
	}

	if (!organizationSyncSettings) {
		return <Loader />;
	}

	const OPTIONS: Option[] = organizations.map((org) => ({
		label: org.name,
		value: org.id,
	}));

	return (
		<>
			<Stack spacing={2}>
				<form onSubmit={form.handleSubmit}>
					<Stack direction="row" alignItems="center">
						<TextField
							{...getFieldHelpers("field")}
							autoFocus
							fullWidth
							label="Organization Sync Field"
							className="w-72"
						/>
						<Switch
							id="organization-assign-default"
							checked={isChecked}
							onCheckedChange={async (checked) => {
								setIsChecked(checked);
								await form.setFieldValue(
									"organization_assign_default",
									checked,
								);
							}}
						/>
						<Label htmlFor="organization-assign-default">
							Assign Default Organization
						</Label>
					</Stack>
					<Stack
						direction="row"
						alignItems="baseline"
						justifyContent="space-between"
						css={styles.tableInfo}
					>
						{/* <ExportPolicyButton
								syncSettings={groupSyncSettings}
								organization={organization}
								type="groups"
							/> */}
					</Stack>

					<div className="flex flex-row py-10 gap-2 justify-between">
						<TextField
							autoFocus
							fullWidth
							label="Idp organization name"
							className="min-w-72 w-72"
						/>
						<MultipleSelector
							className="min-w-96 max-w-3xl"
							value={coderOrgs}
							onChange={setCoderOrgs}
							defaultOptions={OPTIONS}
							hidePlaceholderWhenSelected
							placeholder="Select Coder organizations"
							emptyIndicator={
								<p className="text-center text-lg leading-10 text-content-primary">
									no results found.
								</p>
							}
						/>
						<Button
							className="mt-px"
							onClick={(event) => {
								console.log("add Idp organization");
							}}
						>
							<Plus size={14} />
							Add IdP organization
						</Button>
					</div>

					<Stack spacing={6}>
						<IdpMappingTable isEmpty={organizationMappingCount === 0}>
							{organizationSyncSettings?.mapping &&
								Object.entries(organizationSyncSettings.mapping)
									.sort()
									.map(([idpOrg, organizations]) => (
										<OrganizationRow
											key={idpOrg}
											idpOrg={idpOrg}
											coderOrgs={organizations}
										/>
									))}
						</IdpMappingTable>
						<Button
							className="w-20"
							type="submit"
							// onClick={(event) => {
							// 	event.preventDefault();
							// 	form.handleSubmit();
							// }}
						>
							Save
						</Button>
					</Stack>
				</form>
			</Stack>
		</>
	);
};

interface IdpMappingTableProps {
	isEmpty: boolean;
	children: React.ReactNode;
}

const IdpMappingTable: FC<IdpMappingTableProps> = ({ isEmpty, children }) => {
	const isLoading = false;

	return (
		<TableContainer>
			<Table>
				<TableHead>
					<TableRow>
						<TableCell width="45%">IdP organization</TableCell>
						<TableCell width="55%">Coder organization</TableCell>
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
										message={"No organization mappings"}
										isCompact
										cta={
											<Button variant="outline" asChild>
												<a
													href={docs("/admin/users/idp-sync")}
													className="no-underline"
												>
													<SquareArrowOutUpRight size={14} />
													How to setup IdP organization sync
												</a>
											</Button>
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
	);
};

interface OrganizationRowProps {
	idpOrg: string;
	coderOrgs: readonly string[];
}

const OrganizationRow: FC<OrganizationRowProps> = ({ idpOrg, coderOrgs }) => {
	return (
		<TableRow data-testid={`group-${idpOrg}`}>
			<TableCell>{idpOrg}</TableCell>
			<TableCell>
				<IdpPillList roles={coderOrgs} />
			</TableCell>
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
	fieldText: {
		fontFamily: MONOSPACE_FONT_FAMILY,
		whiteSpace: "nowrap",
		paddingBottom: ".02rem",
	},
	fieldLabel: (theme) => ({
		color: theme.palette.text.secondary,
	}),
	tableInfo: () => ({
		marginBottom: 16,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default IdpSyncPageView;
