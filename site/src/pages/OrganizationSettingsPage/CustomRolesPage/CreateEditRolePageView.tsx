import { useFormik } from "formik";
import { type FC, useId, useState } from "react";
import { useNavigate } from "react-router";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import { RBACResourceActions } from "#/api/rbacresourcesGenerated";
import type {
	AssignableRoles,
	CustomRoleRequest,
	Permission,
	RBACAction,
	RBACResource,
	Role,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { FormFields, FormFooter, VerticalForm } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Label } from "#/components/Label/Label";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { useAITasksEnabled } from "#/modules/tasks/useAITasksEnabled";
import { getFormHelpers, nameValidator } from "#/utils/formUtils";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
});

type CreateEditRolePageViewProps = {
	role: AssignableRoles | undefined;
	onSubmit: (data: CustomRoleRequest) => void;
	error?: unknown;
	isLoading: boolean;
	organizationName: string;
	allResources?: boolean;
};

const CreateEditRolePageView: FC<CreateEditRolePageViewProps> = ({
	role,
	onSubmit,
	error,
	isLoading,
	organizationName,
	allResources = false,
}) => {
	const navigate = useNavigate();
	const onCancel = () => navigate(-1);

	const form = useFormik<CustomRoleRequest>({
		initialValues: {
			name: role?.name || "",
			display_name: role?.display_name || "",
			site_permissions: role?.site_permissions ?? [],
			user_permissions: role?.user_permissions ?? [],
			organization_permissions: role?.organization_permissions ?? [],
			organization_member_permissions:
				role?.organization_member_permissions ?? [],
		},
		validationSchema,
		onSubmit,
	});

	const getFieldHelpers = getFormHelpers<Role>(form, error);

	return (
		<>
			<div className="flex flex-row gap-4 items-baseline justify-between">
				<SettingsHeader>
					<SettingsHeaderTitle>
						{role ? "Edit" : "Create"} Custom Role
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Set a name and permissions for this role.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<div className="flex space-x-2 items-center">
					<Button
						variant="outline"
						onClick={() => {
							navigate(`/organizations/${organizationName}/roles`);
						}}
					>
						Cancel
					</Button>
					<Button
						onClick={() => {
							form.handleSubmit();
						}}
					>
						<Spinner loading={isLoading} />
						{role !== undefined ? "Save" : "Create Role"}
					</Button>
				</div>
			</div>

			<VerticalForm onSubmit={form.handleSubmit}>
				<FormFields>
					{Boolean(error) && !isApiValidationError(error) && (
						<ErrorAlert error={error} />
					)}

					<FormField
						field={getFieldHelpers("name", {
							helperText:
								"The role name cannot be modified after the role is created.",
						})}
						label="Name"
						autoFocus
						disabled={role !== undefined}
						className="w-full"
					/>
					<FormField
						field={getFieldHelpers("display_name", {
							helperText: "Optional: keep empty to default to the name.",
						})}
						label="Display Name"
						className="w-full"
					/>
					<ActionCheckboxes
						permissions={role?.organization_permissions || []}
						form={form}
						allResources={allResources}
					/>
				</FormFields>
				<FormFooter>
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>

					<Button type="submit" disabled={isLoading}>
						<Spinner loading={isLoading} />
						{role ? "Save role" : "Create Role"}
					</Button>
				</FormFooter>
			</VerticalForm>
		</>
	);
};

const ResourceActionComparator = (
	p: Permission,
	resource: string,
	action: string,
) =>
	p.resource_type === resource &&
	(p.action.toString() === "*" || p.action === action);

// the subset of resources that are useful for most users
const DEFAULT_RESOURCES = [
	"audit_log",
	"group",
	"template",
	"organization_member",
	"provisioner_daemon",
	"workspace",
	"idpsync_settings",
];

const resources = new Set(DEFAULT_RESOURCES);

const filteredRBACResourceActions = Object.fromEntries(
	Object.entries(RBACResourceActions).filter(([resource]) =>
		resources.has(resource),
	),
);

// Object.entries widens keys to `string`; this narrows them back to the
// RBACResource union without an assertion.
function isRBACResource(resource: string): resource is RBACResource {
	return resource in RBACResourceActions;
}

interface ActionCheckboxesProps {
	permissions: readonly Permission[];
	form: ReturnType<typeof useFormik<Role>> & { values: Role };
	allResources: boolean;
}

const ActionCheckboxes: FC<ActionCheckboxesProps> = ({
	permissions,
	form,
	allResources,
}) => {
	const [checkedActions, setCheckActions] = useState(permissions);
	const [showAllResources, setShowAllResources] = useState(allResources);
	const aiTasksEnabled = useAITasksEnabled();

	// `RBACResourceActions` is generated at module scope, so the `task` resource
	// is removed here rather than at the constant to keep Tasks roles unsettable
	// while Tasks is disabled.
	const allActions = showAllResources
		? RBACResourceActions
		: filteredRBACResourceActions;
	const resourceActions = aiTasksEnabled
		? allActions
		: Object.fromEntries(
				Object.entries(allActions).filter(([resource]) => resource !== "task"),
			);

	const handleActionCheckChange = async (name: string, checked: boolean) => {
		const [resource_type, action] = name.split(":");

		const newPermissions = checked
			? [
					...checkedActions,
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

	const handleResourceCheckChange = async (
		resource: RBACResource,
		checked: boolean,
		indeterminate: boolean,
	) => {
		const resourceActionsForResource = resourceActions[resource] || {};

		const newCheckedActions =
			!checked || indeterminate
				? checkedActions?.filter((p) => p.resource_type !== resource)
				: checkedActions;

		const newPermissions =
			checked || indeterminate
				? [
						...newCheckedActions,
						...Object.keys(resourceActionsForResource).map((resourceKey) => ({
							negate: false,
							resource_type: resource as RBACResource,
							action: resourceKey as RBACAction,
						})),
					]
				: [...newCheckedActions];

		setCheckActions(newPermissions);
		await form.setFieldValue("organization_permissions", newPermissions);
	};

	return (
		<>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead>Permission</TableHead>
						<TableHead className="py-1 text-right">
							<ShowAllResourcesSwitch
								showAllResources={showAllResources}
								setShowAllResources={setShowAllResources}
							/>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{Object.entries(resourceActions).map(([resourceKey, value]) => {
						if (!isRBACResource(resourceKey)) {
							return null;
						}
						return (
							<PermissionCheckboxGroup
								key={resourceKey}
								checkedActions={checkedActions?.filter(
									(a) => a.resource_type === resourceKey,
								)}
								resourceKey={resourceKey}
								value={value}
								handleActionCheckChange={handleActionCheckChange}
								handleResourceCheckChange={handleResourceCheckChange}
							/>
						);
					})}
				</TableBody>
			</Table>
			<ShowAllResourcesSwitch
				showAllResources={showAllResources}
				setShowAllResources={setShowAllResources}
			/>
		</>
	);
};

interface PermissionCheckboxGroupProps {
	checkedActions: readonly Permission[];
	resourceKey: RBACResource;
	value: Partial<Record<RBACAction, string>>;
	handleActionCheckChange: (name: string, checked: boolean) => Promise<void>;
	handleResourceCheckChange: (
		resource: RBACResource,
		checked: boolean,
		indeterminate: boolean,
	) => Promise<void>;
}

const PermissionCheckboxGroup: FC<PermissionCheckboxGroupProps> = ({
	checkedActions,
	resourceKey,
	value,
	handleActionCheckChange,
	handleResourceCheckChange,
}) => {
	const actionCount = Object.keys(value).length;
	const isResourceChecked = checkedActions.length === actionCount;
	const isResourceIndeterminate =
		checkedActions.length > 0 && checkedActions.length < actionCount;

	return (
		<TableRow key={resourceKey}>
			<TableCell className="px-4" colSpan={2}>
				<li key={resourceKey} className="m-0 list-none">
					<div className="inline-flex items-center gap-2">
						<Checkbox
							name={resourceKey}
							checked={
								isResourceIndeterminate ? "indeterminate" : isResourceChecked
							}
							data-testid={resourceKey}
							aria-label={resourceKey}
							onCheckedChange={(checked) =>
								handleResourceCheckChange(
									resourceKey,
									checked === true,
									isResourceIndeterminate,
								)
							}
						/>
						<span>{resourceKey}</span>
					</div>
					<ul className="m-0 list-none py-2 flex flex-col gap-2 pl-8">
						{Object.entries(value).map(([actionKey, description]) => {
							const actionName = `${resourceKey}:${actionKey}`;
							const isActionChecked = checkedActions.some((p) =>
								ResourceActionComparator(p, resourceKey, actionKey),
							);

							return (
								<li key={actionKey} className="grid grid-cols-[270px_1fr]">
									<span className="inline-flex items-center text-content-primary gap-2">
										<Checkbox
											name={actionName}
											checked={isActionChecked}
											aria-label={actionName}
											onCheckedChange={(checked) =>
												handleActionCheckChange(actionName, checked === true)
											}
										/>
										{actionKey}
									</span>
									<span className="pt-1.5 text-content-secondary">
										{description}
									</span>
								</li>
							);
						})}
					</ul>
				</li>
			</TableCell>
		</TableRow>
	);
};

interface ShowAllResourcesSwitchProps {
	showAllResources: boolean;
	setShowAllResources: React.Dispatch<React.SetStateAction<boolean>>;
}

const ShowAllResourcesSwitch: FC<ShowAllResourcesSwitchProps> = ({
	showAllResources,
	setShowAllResources,
}) => {
	const id = useId();

	return (
		<div className="mr-2 inline-flex items-center justify-end gap-2">
			<Label htmlFor={id} className="cursor-pointer text-xs font-normal">
				{showAllResources
					? "Hide advanced permissions"
					: "Show advanced permissions"}
			</Label>
			<Switch
				id={id}
				size="sm"
				name="show_all_permissions"
				checked={showAllResources}
				onCheckedChange={setShowAllResources}
			/>
		</div>
	);
};

export default CreateEditRolePageView;
