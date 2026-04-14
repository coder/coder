import { useFormik } from "formik";
import { UserIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import * as Yup from "yup";
import { hasApiFieldErrors, isApiError } from "#/api/errors";
import type {
	AssignableRoles,
	LoginType,
	SlimRole,
	UpdateUserProfileRequest,
	User,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { CollapsibleSummary } from "#/components/CollapsibleSummary/CollapsibleSummary";
import { FormFooter } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { FullPageForm } from "#/components/FullPageForm/FullPageForm";
import {
	HelpPopover,
	HelpPopoverContent,
	HelpPopoverIconTrigger,
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { Spinner } from "#/components/Spinner/Spinner";
import { roleDescriptions } from "#/modules/users/roleDescriptions";
import {
	displayNameValidator,
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

const validationSchema = Yup.object({
	username: nameValidator("Username"),
	name: displayNameValidator("Full name"),
});

interface EditUserFormProps {
	error?: unknown;
	isLoading: boolean;
	initialValues: UpdateUserProfileRequest;
	onSubmit: (values: UpdateUserProfileRequest) => void;
	onCancel: () => void;
	user?: User;
	availableRoles?: AssignableRoles[];
	canEditRoles?: boolean;
	oidcRoleSyncEnabled?: boolean;
	isUpdatingRoles?: boolean;
	onUpdateRoles?: (roles: string[]) => void;
}

export const EditUserForm: FC<EditUserFormProps> = ({
	error,
	isLoading,
	initialValues,
	onSubmit,
	onCancel,
	user: userData,
	availableRoles,
	canEditRoles,
	oidcRoleSyncEnabled,
	isUpdatingRoles,
	onUpdateRoles,
}) => {
	const form = useFormik<UpdateUserProfileRequest>({
		initialValues,
		validationSchema,
		onSubmit,
		enableReinitialize: true,
	});

	const getFieldHelpers = getFormHelpers(form, error);

	return (
		<FullPageForm title="Edit user">
			{isApiError(error) && !hasApiFieldErrors(error) && (
				<ErrorAlert error={error} className="mb-8" />
			)}
			<form onSubmit={form.handleSubmit} autoComplete="off">
				<div className="flex flex-col gap-6">
					<FormField
						field={getFieldHelpers("username")}
						label="Username"
						id="username"
						name="username"
						value={form.values.username}
						onChange={onChangeTrimmed(form)}
						onBlur={form.handleBlur}
						autoComplete="username"
						autoFocus
					/>

					<FormField
						field={getFieldHelpers("name")}
						label={
							<>
								Full name{" "}
								<span className="font-normal text-content-secondary">
									(optional)
								</span>
							</>
						}
						id="name"
						name="name"
						value={form.values.name}
						onChange={form.handleChange}
						onBlur={form.handleBlur}
						autoComplete="name"
					/>
				</div>

				<FormFooter className="mt-8">
					<Button onClick={onCancel} variant="outline">
						Cancel
					</Button>
					<Button type="submit" disabled={isLoading}>
						<Spinner loading={isLoading} />
						Save
					</Button>
				</FormFooter>
			</form>

			{canEditRoles && availableRoles && userData && onUpdateRoles && (
				<RolesSection
					availableRoles={availableRoles}
					userRoles={userData.roles}
					userLoginType={userData.login_type}
					oidcRoleSyncEnabled={oidcRoleSyncEnabled ?? false}
					isUpdatingRoles={isUpdatingRoles ?? false}
					onUpdateRoles={onUpdateRoles}
				/>
			)}
		</FullPageForm>
	);
};

interface RolesSectionProps {
	availableRoles: AssignableRoles[];
	userRoles: readonly SlimRole[];
	userLoginType: LoginType;
	oidcRoleSyncEnabled: boolean;
	isUpdatingRoles: boolean;
	onUpdateRoles: (roles: string[]) => void;
}

const RolesSection: FC<RolesSectionProps> = ({
	availableRoles,
	userRoles,
	userLoginType,
	oidcRoleSyncEnabled,
	isUpdatingRoles,
	onUpdateRoles,
}) => {
	const selectedRoleNames = new Set(userRoles.map((r) => r.name));
	const canSetRoles =
		userLoginType !== "oidc" ||
		(userLoginType === "oidc" && !oidcRoleSyncEnabled);

	const handleChange = (roleName: string) => {
		const next = new Set(selectedRoleNames);
		if (next.has(roleName)) {
			next.delete(roleName);
		} else {
			next.add(roleName);
		}
		// Remove the synthetic "member" fallback if present.
		next.delete("member");
		onUpdateRoles([...next]);
	};

	const filteredRoles = availableRoles.filter(
		(role) => role.name !== "organization-workspace-creation-ban",
	);
	const advancedRoles = availableRoles.filter(
		(role) => role.name === "organization-workspace-creation-ban",
	);

	const [isAdvancedOpen, setIsAdvancedOpen] = useState(false);
	useEffect(() => {
		if (selectedRoleNames.has("organization-workspace-creation-ban")) {
			setIsAdvancedOpen(true);
		}
	}, [selectedRoleNames]);

	return (
		<div className="mt-10 border-0 border-t border-solid border-border pt-8">
			<div className="flex items-center gap-2 mb-6">
				<h3 className="text-lg font-medium m-0">Roles</h3>
				{!canSetRoles && (
					<HelpPopover>
						<HelpPopoverIconTrigger size="small" />
						<HelpPopoverContent>
							<HelpPopoverTitle>Externally controlled</HelpPopoverTitle>
							<HelpPopoverText>
								Roles for this user are controlled by the OIDC identity
								provider.
							</HelpPopoverText>
						</HelpPopoverContent>
					</HelpPopover>
				)}
			</div>

			{canSetRoles ? (
				<fieldset
					className="border-0 m-0 p-0 disabled:opacity-50"
					disabled={isUpdatingRoles}
					title="Available roles"
				>
					<div className="flex flex-col gap-4">
						{filteredRoles.map((role) => (
							<RoleOption
								key={role.name}
								value={role.name}
								name={role.display_name || role.name}
								description={roleDescriptions[role.name] ?? ""}
								isChecked={selectedRoleNames.has(role.name)}
								onChange={handleChange}
							/>
						))}
						{advancedRoles.length > 0 && (
							<CollapsibleSummary label="advanced" defaultOpen={isAdvancedOpen}>
								{advancedRoles.map((role) => (
									<RoleOption
										key={role.name}
										value={role.name}
										name={role.display_name || role.name}
										description={roleDescriptions[role.name] ?? ""}
										isChecked={selectedRoleNames.has(role.name)}
										onChange={handleChange}
									/>
								))}
							</CollapsibleSummary>
						)}
					</div>

					<div className="mt-6 pt-4 border-0 border-t border-solid border-border text-sm">
						<div className="flex gap-4">
							<UserIcon className="size-5 shrink-0 mt-0.5" />
							<div className="flex flex-col">
								<strong>Member</strong>
								<span className="text-xs text-content-secondary">
									{roleDescriptions.member}
								</span>
							</div>
						</div>
					</div>
				</fieldset>
			) : (
				<p className="text-sm text-content-secondary">
					Roles for this user are controlled by the OIDC identity provider and
					cannot be changed here.
				</p>
			)}
		</div>
	);
};

interface RoleOptionProps {
	value: string;
	name: string;
	description: string;
	isChecked: boolean;
	onChange: (roleName: string) => void;
}

const RoleOption: FC<RoleOptionProps> = ({
	value,
	name,
	description,
	isChecked,
	onChange,
}) => {
	return (
		<label htmlFor={name} className="cursor-pointer">
			<div className="flex items-start gap-4">
				<Checkbox
					id={name}
					checked={isChecked}
					onCheckedChange={() => {
						onChange(value);
					}}
				/>
				<div className="flex flex-col">
					<strong className="text-sm">{name}</strong>
					<span className="text-xs text-content-secondary">{description}</span>
				</div>
			</div>
		</label>
	);
};
