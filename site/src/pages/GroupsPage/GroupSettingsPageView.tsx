import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type { Group } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { IconField } from "#/components/IconField/IconField";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { isEveryoneGroup } from "#/modules/groups";
import { usdBudgetFormatter } from "#/utils/currency";
import {
	getFormHelpers,
	nameValidator,
	onChangeTrimmed,
} from "#/utils/formUtils";

type FormData = {
	name: string;
	display_name: string;
	avatar_url: string;
	quota_allowance: number;
	// Per-member AI budget, in dollars. "" is no budget (uncapped); 0 disables.
	monthly_budget_per_member: string;
};

const validationSchema = Yup.object({
	name: nameValidator("Name"),
	quota_allowance: Yup.number().required().min(0).integer(),
	// Optional: empty is uncapped. A value must be zero or more (0 disables).
	monthly_budget_per_member: Yup.number()
		.transform((value, original) => (original === "" ? undefined : value))
		.min(0, "Enter an amount of zero or more."),
});

interface UpdateGroupFormProps {
	group: Group;
	/** Whether the AI add-on settings are shown (gated by the aibridge feature). */
	showAISettings: boolean;
	/** Per-member AI budget in dollars, or null when none is set. */
	initialBudgetDollars: number | null;
	errors: unknown;
	onSubmit: (data: FormData) => void;
	onCancel: () => void;
	isLoading: boolean;
}

const UpdateGroupForm: FC<UpdateGroupFormProps> = ({
	group,
	showAISettings,
	initialBudgetDollars,
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
			monthly_budget_per_member:
				initialBudgetDollars === null ? "" : String(initialBudgetDollars),
		},
		validationSchema,
		onSubmit,
	});
	const getFieldHelpers = getFormHelpers<FormData>(form, errors);
	const nameField = getFieldHelpers("name");
	const displayNameField = getFieldHelpers("display_name", {
		helperText: "Keep empty to default to the name.",
	});
	const quotaField = getFieldHelpers("quota_allowance", {
		helperText: `This group gives ${form.values.quota_allowance} quota credits to each
            of its members.`,
	});
	const budgetField = getFieldHelpers("monthly_budget_per_member");
	const budgetDollars = form.values.monthly_budget_per_member;
	const memberCount = group.total_member_count;
	const monthlyMaximum = usdBudgetFormatter.format(
		Number(budgetDollars) * memberCount,
	);

	return (
		<form className="flex flex-col gap-10 pb-8" onSubmit={form.handleSubmit}>
			<section className="flex flex-col gap-4 max-w-md">
				<div className="flex flex-col gap-2">
					<h2 className="text-xl font-semibold text-content-primary m-0">
						General
					</h2>
				</div>
				<div className="flex flex-col gap-6">
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={nameField.id}>Name</Label>
						<Input
							id={nameField.id}
							name={nameField.name}
							value={nameField.value}
							onChange={onChangeTrimmed(form)}
							onBlur={nameField.onBlur}
							autoComplete="name"
							autoFocus
							disabled={isEveryoneGroup(group)}
							aria-invalid={nameField.error}
						/>
						{nameField.helperText && (
							<span
								className={`text-xs text-left ${
									nameField.error
										? "text-content-destructive"
										: "text-content-secondary"
								}`}
							>
								{nameField.helperText}
							</span>
						)}
					</div>
					{!isEveryoneGroup(group) && (
						<>
							<div className="flex flex-col items-start gap-2">
								<Label htmlFor={displayNameField.id}>Display name</Label>
								<Input
									id={displayNameField.id}
									name={displayNameField.name}
									value={displayNameField.value}
									onChange={displayNameField.onChange}
									onBlur={displayNameField.onBlur}
									autoComplete="display_name"
									disabled={isEveryoneGroup(group)}
									aria-invalid={displayNameField.error}
								/>
								{displayNameField.helperText && (
									<span
										className={`text-xs text-left ${
											displayNameField.error
												? "text-content-destructive"
												: "text-content-secondary"
										}`}
									>
										{displayNameField.helperText}
									</span>
								)}
							</div>
							<IconField
								{...getFieldHelpers("avatar_url")}
								onChange={onChangeTrimmed(form)}
								fullWidth
								label="Avatar URL"
								onPickEmoji={(value) => form.setFieldValue("avatar_url", value)}
							/>
						</>
					)}
				</div>
			</section>

			{showAISettings && (
				<section className="flex flex-col gap-8 max-w-md">
					<div className="flex items-center gap-2">
						<h2 className="text-xl font-semibold text-content-primary m-0">
							AI budget
						</h2>
						<Badge variant="purple" size="sm">
							AI add-on
						</Badge>
					</div>
					<div className="flex flex-col gap-6">
						<div className="flex flex-col items-start gap-2">
							<Label htmlFor={budgetField.id}>
								Monthly budget per member (USD)
							</Label>
							<Input
								id={budgetField.id}
								name={budgetField.name}
								value={budgetField.value}
								onChange={(event) =>
									form.setFieldValue(budgetField.name, event.target.value)
								}
								onBlur={budgetField.onBlur}
								type="number"
								min="0"
								step="1"
								aria-invalid={budgetField.error}
							/>
							{budgetField.error ? (
								<span className="text-xs text-left text-content-destructive">
									{budgetField.helperText}
								</span>
							) : budgetDollars.trim() !== "" ? (
								<span className="text-xs text-left text-content-secondary">
									<span className="font-medium text-content-primary">
										{monthlyMaximum}
									</span>
									/month maximum, based on{" "}
									<span className="font-medium text-content-primary">
										{memberCount}
									</span>{" "}
									{memberCount === 1 ? "member" : "members"}.
								</span>
							) : (
								<span className="text-xs text-left text-content-secondary">
									Leave empty for uncapped spend.
								</span>
							)}
						</div>
					</div>
				</section>
			)}

			<section className="flex flex-col gap-8">
				<div className="flex flex-col gap-2">
					<h2 className="text-xl font-semibold text-content-primary m-0">
						Quotas
					</h2>
					<p className="text-sm leading-none m-0 text-content-secondary">
						You can use quotas to restrict how many resources a user can create.
					</p>
				</div>
				<div className="flex flex-col gap-6">
					<div className="flex flex-col items-start gap-2">
						<Label htmlFor={quotaField.id}>Quota Allowance</Label>
						<Input
							id={quotaField.id}
							name={quotaField.name}
							value={quotaField.value}
							onChange={onChangeTrimmed(form)}
							onBlur={quotaField.onBlur}
							type="number"
							aria-invalid={quotaField.error}
							className="w-40"
						/>
						{quotaField.helperText && (
							<span
								className={`text-xs text-left ${
									quotaField.error
										? "text-content-destructive"
										: "text-content-secondary"
								}`}
							>
								{quotaField.helperText}
							</span>
						)}
					</div>
				</div>
			</section>

			<footer className="flex items-center justify-start space-x-2">
				<Button onClick={onCancel} variant="outline">
					Cancel
				</Button>

				<Button type="submit" disabled={isLoading}>
					<Spinner loading={isLoading} />
					Save
				</Button>
			</footer>
		</form>
	);
};

type SettingsGroupPageViewProps = {
	onCancel: () => void;
	onSubmit: (data: FormData) => void;
	group: Group;
	showAISettings: boolean;
	initialBudgetDollars: number | null;
	formErrors: unknown;
	isUpdating: boolean;
};

const GroupSettingsPageView: FC<SettingsGroupPageViewProps> = ({
	onCancel,
	onSubmit,
	group,
	showAISettings,
	initialBudgetDollars,
	formErrors,
	isUpdating,
}) => {
	return (
		<UpdateGroupForm
			group={group}
			showAISettings={showAISettings}
			initialBudgetDollars={initialBudgetDollars}
			onCancel={onCancel}
			errors={formErrors}
			isLoading={isUpdating}
			onSubmit={onSubmit}
		/>
	);
};

export default GroupSettingsPageView;
