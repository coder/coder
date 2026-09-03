import { type FormikContextType, useFormik } from "formik";
import { ArrowLeftIcon } from "lucide-react";
import {
	type Dispatch,
	type FC,
	type SetStateAction,
	useId,
	useState,
} from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import { isApiValidationError } from "#/api/errors";
import { RBACResourceActions } from "#/api/rbacresourcesGenerated";
import {
	type AssignableRoles,
	type CustomRoleRequest,
	type Permission,
	type RBACAction,
	RBACActions,
	type RBACResource,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { FormFooter } from "#/components/Form/Form";
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
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	display_name: displayNameValidator("Display name"),
});

type CreateEditRolePageViewProps = {
	role: AssignableRoles | undefined;
	onSubmit: (data: CustomRoleRequest) => void;
	error?: unknown;
	isLoading: boolean;
	organizationName: string;
	allResources?: boolean;
};

export const CreateEditRolePageView: FC<CreateEditRolePageViewProps> = ({
	role,
	onSubmit,
	error,
	isLoading,
	organizationName,
	allResources = false,
}) => {
	const isEditing = role !== undefined;
	const rolesHref = `/organizations/${organizationName}/roles`;
	const form = useFormik<CustomRoleRequest>({
		initialValues: {
			name: role?.name ?? "",
			display_name: role?.display_name ?? "",
			site_permissions: role?.site_permissions ?? [],
			user_permissions: role?.user_permissions ?? [],
			organization_permissions: role?.organization_permissions ?? [],
			organization_member_permissions:
				role?.organization_member_permissions ?? [],
		},
		validationSchema,
		onSubmit,
	});

	const getFieldHelpers = getFormHelpers<CustomRoleRequest>(form, error);

	return (
		<div>
			<Button variant="subtle" asChild className="-ml-3">
				<Link to={rolesHref}>
					<ArrowLeftIcon />
					<span>Back to roles</span>
				</Link>
			</Button>

			<div className="pt-6">
				<SettingsHeader>
					<SettingsHeaderTitle>
						{isEditing ? "Edit Custom Role" : "New Custom Role"}
					</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Set a name and permissions for this role.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<div className="border border-solid p-6 rounded-lg">
					<form
						onSubmit={form.handleSubmit}
						noValidate
						autoComplete="off"
						aria-label="Custom role settings form"
						className="flex flex-col gap-6"
					>
						<fieldset
							disabled={isLoading}
							className="flex flex-col gap-6 m-0 border-none p-0 min-w-0"
						>
							{Boolean(error) && !isApiValidationError(error) && (
								<ErrorAlert error={error} />
							)}

							<FormField
								field={getFieldHelpers("name", {
									helperText: "Cannot be changed after the role is created.",
								})}
								label="Name"
								required
								autoFocus
								disabled={isEditing}
								onChange={onChangeTrimmed(form)}
								className="w-full"
							/>
							<FormField
								field={getFieldHelpers("display_name", {
									helperText: "Keep empty to default to the name.",
								})}
								label="Display name"
								className="w-full"
							/>
							<ActionCheckboxes
								permissions={role?.organization_permissions ?? []}
								form={form}
								allResources={allResources}
							/>
						</fieldset>

						<FormFooter>
							<Button asChild variant="outline">
								<Link to={rolesHref}>Cancel</Link>
							</Button>
							<Button type="submit" disabled={isLoading}>
								<Spinner loading={isLoading} aria-hidden />
								{isEditing ? "Save" : "Create custom role"}
							</Button>
						</FormFooter>
					</form>
				</div>
			</div>
		</div>
	);
};

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
	"idpsync_settings",
];

const resources = new Set(DEFAULT_RESOURCES);

const filteredRBACResourceActions = Object.fromEntries(
	Object.entries(RBACResourceActions).filter(([resource]) =>
		resources.has(resource),
	),
);

function isRBACResource(resource: string): resource is RBACResource {
	return resource in RBACResourceActions;
}

function isRBACAction(action: string): action is RBACAction {
	return RBACActions.some((rbacAction) => rbacAction === action);
}

interface ActionCheckboxesProps {
	permissions: readonly Permission[];
	form: FormikContextType<CustomRoleRequest>;
	allResources: boolean;
}

const ActionCheckboxes: FC<ActionCheckboxesProps> = ({
	permissions,
	form,
	allResources,
}) => {
	const [checkedActions, setCheckActions] = useState(permissions);
	const [showAllResources, setShowAllResources] = useState(allResources);

	const resourceActions = showAllResources
		? RBACResourceActions
		: filteredRBACResourceActions;

	const handleActionCheckChange = async (name: string, checked: boolean) => {
		const [resourceType, action] = name.split(":");
		if (!isRBACResource(resourceType) || !isRBACAction(action)) {
			return;
		}

		const newPermissions = checked
			? [
					...checkedActions,
					{
						negate: false,
						resource_type: resourceType,
						action,
					},
				]
			: checkedActions.filter(
					(p) => p.resource_type !== resourceType || p.action !== action,
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
				? checkedActions.filter((p) => p.resource_type !== resource)
				: checkedActions;

		const resourcePermissions: Permission[] = [];
		if (checked || indeterminate) {
			for (const resourceKey of Object.keys(resourceActionsForResource)) {
				if (!isRBACAction(resourceKey)) {
					continue;
				}
				resourcePermissions.push({
					negate: false,
					resource_type: resource,
					action: resourceKey,
				});
			}
		}

		const newPermissions = [...newCheckedActions, ...resourcePermissions];

		setCheckActions(newPermissions);
		await form.setFieldValue("organization_permissions", newPermissions);
	};

	return (
		<Table aria-label="Role permissions">
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
							checkedActions={checkedActions.filter(
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
		<TableRow>
			<TableCell className="px-4" colSpan={2}>
				<div>
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
				</div>
			</TableCell>
		</TableRow>
	);
};

interface ShowAllResourcesSwitchProps {
	showAllResources: boolean;
	setShowAllResources: Dispatch<SetStateAction<boolean>>;
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
