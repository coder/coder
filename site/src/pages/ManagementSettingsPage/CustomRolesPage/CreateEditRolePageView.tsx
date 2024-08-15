import type { Interpolation, Theme } from "@emotion/react";
import VisibilityOffOutlinedIcon from "@mui/icons-material/VisibilityOffOutlined";
import VisibilityOutlinedIcon from "@mui/icons-material/VisibilityOutlined";
import Button from "@mui/material/Button";
import Checkbox from "@mui/material/Checkbox";
import FormControlLabel from "@mui/material/FormControlLabel";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableFooter from "@mui/material/TableFooter";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import TextField from "@mui/material/TextField";
import { isApiValidationError } from "api/errors";
import { RBACResourceActions } from "api/rbacresources_gen";
import type {
	AssignableRoles,
	CustomRoleRequest,
	Permission,
	RBACAction,
	RBACResource,
	Role,
} from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { FormFields, FormFooter, VerticalForm } from "components/Form/Form";
import { SettingsHeader } from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { useFormik } from "formik";
import { type ChangeEvent, type FC, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getFormHelpers, nameValidator } from "utils/formUtils";
import * as Yup from "yup";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
});

export type CreateEditRolePageViewProps = {
	role: AssignableRoles | undefined;
	onSubmit: (data: CustomRoleRequest) => void;
	error?: unknown;
	isLoading: boolean;
	organizationName: string;
	canAssignOrgRole: boolean;
	allResources?: boolean;
};

export const CreateEditRolePageView: FC<CreateEditRolePageViewProps> = ({
	role,
	onSubmit,
	error,
	isLoading,
	organizationName,
	canAssignOrgRole,
	allResources = false,
}) => {
	const navigate = useNavigate();
	const onCancel = () => navigate(-1);

	const form = useFormik<CustomRoleRequest>({
		initialValues: {
			name: role?.name || "",
			display_name: role?.display_name || "",
			site_permissions: role?.site_permissions || [],
			organization_permissions: role?.organization_permissions || [],
			user_permissions: role?.user_permissions || [],
		},
		validationSchema,
		onSubmit,
	});

	const getFieldHelpers = getFormHelpers<Role>(form, error);

	return (
		<>
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
				<SettingsHeader
					title={`${role ? "Edit" : "Create"} Custom Role`}
					description="Set a name and permissions for this role."
				/>
				{canAssignOrgRole && (
					<Stack direction="row" spacing={2}>
						<Button
							onClick={() => {
								navigate(`/organizations/${organizationName}/roles`);
							}}
						>
							Cancel
						</Button>
						<Button
							variant="contained"
							color="primary"
							onClick={() => {
								form.handleSubmit();
							}}
						>
							{role !== undefined ? "Save" : "Create Role"}
						</Button>
					</Stack>
				)}
			</Stack>

			<VerticalForm onSubmit={form.handleSubmit}>
				<FormFields>
					{Boolean(error) && !isApiValidationError(error) && (
						<ErrorAlert error={error} />
					)}

					<TextField
						{...getFieldHelpers("name", {
							helperText:
								"The role name cannot be modified after the role is created.",
						})}
						autoFocus
						fullWidth
						disabled={role !== undefined}
						label="Name"
					/>
					<TextField
						{...getFieldHelpers("display_name", {
							helperText: "Optional: keep empty to default to the name.",
						})}
						fullWidth
						label="Display Name"
					/>
					<ActionCheckboxes
						permissions={role?.organization_permissions || []}
						form={form}
						allResources={allResources}
					/>
				</FormFields>
				{canAssignOrgRole && (
					<FormFooter
						onCancel={onCancel}
						isLoading={isLoading}
						submitLabel={role !== undefined ? "Save" : "Create Role"}
					/>
				)}
			</VerticalForm>
		</>
	);
};

interface ActionCheckboxesProps {
	permissions: readonly Permission[] | undefined;
	form: ReturnType<typeof useFormik<Role>> & { values: Role };
	allResources: boolean;
}

const ResourceActionComparator = (
	p: Permission,
	resource: string,
	action: string,
) =>
	p.resource_type === resource &&
	(p.action.toString() === "*" || p.action === action);

const DEFAULT_RESOURCES = [
	"audit_log",
	"group",
	"template",
	"organization_member",
	"provisioner_daemon",
	"workspace",
];

const resources = new Set(DEFAULT_RESOURCES);

const filteredRBACResourceActions = Object.fromEntries(
	Object.entries(RBACResourceActions).filter(([resource]) =>
		resources.has(resource),
	),
);

const ActionCheckboxes: FC<ActionCheckboxesProps> = ({
	permissions,
	form,
	allResources,
}) => {
	const [checkedActions, setCheckActions] = useState(permissions);
	const [showAllResources, setShowAllResources] = useState(allResources);

	const handleActionCheckChange = async (
		e: ChangeEvent<HTMLInputElement>,
		form: ReturnType<typeof useFormik<Role>> & { values: Role },
	) => {
		const { name, checked } = e.currentTarget;
		const [resource_type, action] = name.split(":");

		const newPermissions = checked
			? [
					...(checkedActions ?? []),
					{
						negate: false,
						resource_type: resource_type as RBACResource,
						action: action as RBACAction,
					},
				]
			: checkedActions?.filter(
					(p) => p.resource_type !== resource_type || p.action !== action,
				);

		setCheckActions(newPermissions);
		await form.setFieldValue("organization_permissions", newPermissions);
	};

	const resourceActions = showAllResources
		? RBACResourceActions
		: filteredRBACResourceActions;

	return (
		<TableContainer>
			<Table>
				<TableHead>
					<TableRow>
						<TableCell>Permission</TableCell>
						<TableCell
							align="right"
							sx={{ paddingTop: 0.4, paddingBottom: 0.4 }}
						>
							<ShowAllResourcesCheckbox
								showAllResources={showAllResources}
								setShowAllResources={setShowAllResources}
							/>
						</TableCell>
					</TableRow>
				</TableHead>
				<TableBody>
					{Object.entries(resourceActions).map(([resourceKey, value]) => {
						return (
							<TableRow key={resourceKey}>
								<TableCell sx={{ paddingLeft: 2 }} colSpan={2}>
									<li key={resourceKey} css={styles.checkBoxes}>
										{resourceKey}
										<ul css={styles.checkBoxes}>
											{Object.entries(value).map(([actionKey, value]) => (
												<li key={actionKey}>
													<span css={styles.actionText}>
														<Checkbox
															size="small"
															name={`${resourceKey}:${actionKey}`}
															checked={checkedActions?.some((p) =>
																ResourceActionComparator(
																	p,
																	resourceKey,
																	actionKey,
																),
															)}
															onChange={(e) => handleActionCheckChange(e, form)}
														/>
														{actionKey}
													</span>{" "}
													&ndash;{" "}
													<span css={styles.actionDescription}>{value}</span>
												</li>
											))}
										</ul>
									</li>
								</TableCell>
							</TableRow>
						);
					})}
				</TableBody>
				<TableFooter>
					<TableRow>
						<TableCell
							align="right"
							colSpan={2}
							sx={{ paddingTop: 0.4, paddingBottom: 0.4, paddingRight: 4 }}
						>
							<ShowAllResourcesCheckbox
								showAllResources={showAllResources}
								setShowAllResources={setShowAllResources}
							/>
						</TableCell>
					</TableRow>
				</TableFooter>
			</Table>
		</TableContainer>
	);
};

interface ShowAllResourcesCheckboxProps {
	showAllResources: boolean;
	setShowAllResources: React.Dispatch<React.SetStateAction<boolean>>;
}

const ShowAllResourcesCheckbox: FC<ShowAllResourcesCheckboxProps> = ({
	showAllResources,
	setShowAllResources,
}) => {
	return (
		<FormControlLabel
			sx={{ marginRight: 1 }}
			control={
				<Checkbox
					size="small"
					id="show_all_permissions"
					name="show_all_permissions"
					checked={showAllResources}
					onChange={(e) => setShowAllResources(e.currentTarget.checked)}
					checkedIcon={<VisibilityOutlinedIcon />}
					icon={<VisibilityOffOutlinedIcon />}
				/>
			}
			label={<span style={{ fontSize: 12 }}>Show all permissions</span>}
		/>
	);
};

const styles = {
	checkBoxes: {
		margin: 0,
		listStyleType: "none",
	},
	actionText: (theme) => ({
		color: theme.palette.text.primary,
	}),
	actionDescription: (theme) => ({
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default CreateEditRolePageView;
