import TextField from "@mui/material/TextField";
import type { Group } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import { IconField } from "components/IconField/IconField";
import { Loader } from "components/Loader/Loader";
import { ResourcePageHeader } from "components/PageHeader/PageHeader";
import { Spinner } from "components/Spinner/Spinner";
import { useFormik } from "formik";
import type { FC } from "react";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "utils/formUtils";
import { isEveryoneGroup } from "utils/groups";
import * as Yup from "yup";

type FormData = {
	name: string;
	display_name: string;
	avatar_url: string;
	quota_allowance: number;
};

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	quota_allowance: Yup.number().required().min(0).integer(),
});

interface UpdateGroupFormProps {
	group: Group;
	errors: unknown;
	onSubmit: (data: FormData) => void;
	onCancel: () => void;
	isLoading: boolean;
}

const UpdateGroupForm: FC<UpdateGroupFormProps> = ({
	group,
	errors,
	onSubmit,
	onCancel,
	isLoading,
}) => {
	const form = useFormik<FormData>({
		initialValues: {
			name: group.name,
			display_name: group.display_name,
			avatar_url: group.avatar_url,
			quota_allowance: group.quota_allowance,
		},
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers<FormData>(form, errors);

	return (
		<HorizontalForm onSubmit={form.handleSubmit}>
			<FormSection
				title="Group settings"
				description="Set a name and avatar for this group."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("name")}
						onChange={onChangeTrimmed(form)}
						autoComplete="name"
						autoFocus
						fullWidth
						label="Name"
						disabled={isEveryoneGroup(group)}
					/>
					{!isEveryoneGroup(group) && (
						<>
							<TextField
								{...getFieldHelpers("display_name", {
									helperText: "Optional: keep empty to default to the name.",
								})}
								autoComplete="display_name"
								autoFocus
								fullWidth
								label="Display Name"
								disabled={isEveryoneGroup(group)}
							/>
							<IconField
								{...getFieldHelpers("avatar_url")}
								onChange={onChangeTrimmed(form)}
								fullWidth
								label="Avatar URL"
								onPickEmoji={(value) => form.setFieldValue("avatar_url", value)}
							/>
						</>
					)}
				</FormFields>
			</FormSection>
			<FormSection
				title="Quota"
				description="You can use quotas to restrict how many resources a user can create."
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("quota_allowance", {
							helperText: `This group gives ${form.values.quota_allowance} quota credits to each
            of its members.`,
						})}
						onChange={onChangeTrimmed(form)}
						autoFocus
						fullWidth
						type="number"
						label="Quota Allowance"
					/>
				</FormFields>
			</FormSection>

			<FormFooter className="mt-8">
				<Button onClick={onCancel} variant="outline">
					Cancel
				</Button>

				<Button type="submit" disabled={isLoading}>
					<Spinner loading={isLoading} />
					Save
				</Button>
			</FormFooter>
		</HorizontalForm>
	);
};

export type SettingsGroupPageViewProps = {
	onCancel: () => void;
	onSubmit: (data: FormData) => void;
	group: Group | undefined;
	formErrors: unknown;
	isLoading: boolean;
	isUpdating: boolean;
};

const GroupSettingsPageView: FC<SettingsGroupPageViewProps> = ({
	onCancel,
	onSubmit,
	group,
	formErrors,
	isLoading,
	isUpdating,
}) => {
	if (isLoading) {
		return <Loader />;
	}

	return (
		<>
			<ResourcePageHeader
				displayName={group!.display_name}
				name={group!.name}
				css={{ paddingTop: 8 }}
			/>
			<UpdateGroupForm
				group={group!}
				onCancel={onCancel}
				errors={formErrors}
				isLoading={isUpdating}
				onSubmit={onSubmit}
			/>
		</>
	);
};

export default GroupSettingsPageView;
