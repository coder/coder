import { useFormik } from "formik";
import { ArrowLeftIcon, CpuIcon, InfoIcon } from "lucide-react";
import { Link, useNavigate, useParams } from "react-router";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { Margins } from "#/components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "#/components/PageHeader/PageHeader";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { getFormHelpers } from "#/utils/formUtils";
import { pageTitle } from "#/utils/page";
import { generateUUID } from "#/utils/random";
import {
	type DemoRunner,
	demoTemplates,
	type RunnerValues,
	runnerValuesSchema,
} from "./demoData";
import { useRunners } from "./RunnersLayout";

const RunnerFormPage: React.FC = () => {
	const { id } = useParams();
	const { runners } = useRunners();
	const runner = runners.find((runner) => runner.id === id);
	if (id && !runner)
		return (
			<Margins>
				<EmptyState
					message="Runner not found"
					cta={
						<Button asChild variant="outline">
							<Link to="/runners">Back to runners</Link>
						</Button>
					}
				/>
			</Margins>
		);
	return <RunnerForm key={id ?? "new"} runner={runner} />;
};

type RunnerFormProps = { runner: DemoRunner | undefined };

const RunnerForm: React.FC<RunnerFormProps> = ({ runner }) => {
	const { runners, saveRunner } = useRunners();
	const navigate = useNavigate();
	const back = runner ? `/runners/${runner.id}` : "/runners";
	const title = runner ? "Edit runner" : "Create new runner";
	const form = useFormik<RunnerValues>({
		initialValues: runner ?? {
			label: "",
			template: "",
			preset: "",
			poolSize: 0,
			maxRunners: 10,
		},
		validationSchema: runnerValuesSchema.test("unique", (values, context) =>
			runners.some(
				(item) =>
					item.id !== runner?.id &&
					item.label.toLowerCase() === values.label?.toLowerCase(),
			)
				? context.createError({
						path: "label",
						message: "This runner label is already in use.",
					})
				: true,
		),
		onSubmit: (values) => {
			const id = runner?.id ?? generateUUID();
			saveRunner({ ...values, id });
			navigate(`/runners/${id}`);
		},
	});
	const field = getFormHelpers(form);
	const template = demoTemplates.find(
		(template) => template.name === form.values.template,
	);
	return (
		<Margins className="pb-16">
			<title>{pageTitle(title)}</title>
			<Button asChild variant="subtle" className="mt-8 -ml-3">
				<Link to={back}>
					<ArrowLeftIcon />
					Back to {runner ? "runner" : "runners"}
				</Link>
			</Button>
			<PageHeader>
				<PageHeaderTitle>{title}</PageHeaderTitle>
				<PageHeaderSubtitle>
					Connect a GitHub Actions runner label to a Coder template.
				</PageHeaderSubtitle>
			</PageHeader>
			<HorizontalForm onSubmit={form.handleSubmit} noValidate>
				<FormSection
					title="Runner configuration"
					description="Choose the label your workflows will use and the template that provisions each runner."
				>
					<FormFields className="max-w-150">
						<FormField
							field={field("label", {
								helperText:
									"Use this label in your GitHub Actions workflow's runs-on configuration.",
							})}
							label="Runner label"
							placeholder="e.g. coder-small"
							required
							autoFocus
						/>
						<FormField
							field={field("template", {
								helperText:
									"Each job runs in a fresh workspace created from this template.",
							})}
							label="Template"
							required
							control={(props) => (
								<Combobox
									value={form.values.template}
									onValueChange={(value) => {
										form.setValues({
											...form.values,
											template: value ?? "",
											preset: "",
										});
									}}
								>
									<ComboboxTrigger asChild>
										<Button
											{...props}
											type="button"
											variant="outline"
											className="h-11 w-full justify-start"
											onBlur={() => form.setFieldTouched("template")}
										>
											{template ? (
												<>
													<Avatar src={template.icon} size="sm" />
													{template.displayName}
													<span className="ml-auto text-xs text-content-secondary">
														{template.name}
													</span>
												</>
											) : (
												<span className="text-content-secondary">
													Select a template...
												</span>
											)}
										</Button>
									</ComboboxTrigger>
									<ComboboxContent
										className="w-(--radix-popover-trigger-width)"
										label="Templates"
									>
										<ComboboxInput
											placeholder="Search templates..."
											aria-label="Search templates"
										/>
										<ComboboxList>
											{demoTemplates.map((item) => (
												<ComboboxItem key={item.name} value={item.name}>
													<Avatar src={item.icon} size="sm" />
													<div className="flex flex-col">
														<span>{item.displayName}</span>
														<span className="text-xs text-content-secondary">
															{item.name}
														</span>
													</div>
												</ComboboxItem>
											))}
											<ComboboxEmpty>No templates found.</ComboboxEmpty>
										</ComboboxList>
									</ComboboxContent>
								</Combobox>
							)}
						/>
						<FormField
							field={field("preset", {
								helperText:
									"Presets define the resources and parameters for your runner.",
							})}
							label="Preset"
							required
							control={(props) => (
								<Select
									value={form.values.preset}
									onValueChange={(value) => form.setFieldValue("preset", value)}
									disabled={!template}
								>
									<SelectTrigger
										{...props}
										onBlur={() => form.setFieldTouched("preset")}
									>
										<SelectValue
											placeholder={
												template
													? "Select a preset..."
													: "Choose a template first"
											}
										/>
									</SelectTrigger>
									<SelectContent>
										{template?.presets.map((preset) => (
											<SelectItem key={preset} value={preset}>
												{preset}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							)}
						/>
						<div className="flex items-center gap-3 rounded-lg border bg-surface-secondary p-4 text-sm">
							<CpuIcon className="size-4 text-content-secondary" />
							<span>Runner platform</span>
							<Badge variant="outline" className="ml-auto">
								Linux x64
							</Badge>
						</div>
					</FormFields>
				</FormSection>
				<FormSection
					title="Pool sizing"
					description="Balance fast job starts with resource usage. Runners scale with demand, up to your configured maximum."
				>
					<FormFields className="max-w-150">
						<div className="grid gap-6 sm:grid-cols-2">
							<FormField
								field={field("poolSize", {
									helperText: "Idle runners kept ready for new jobs.",
								})}
								label="Pool size"
								type="number"
								min={0}
								step={1}
								required
							/>
							<FormField
								field={field("maxRunners", {
									helperText:
										"Maximum total runners, including starting runners.",
								})}
								label="Maximum runners"
								type="number"
								min={1}
								step={1}
								required
							/>
						</div>
						<div className="flex items-start gap-3 rounded-lg border p-4 text-sm text-content-secondary">
							<InfoIcon className="mt-0.5 size-4 shrink-0" />
							<p className="m-0 leading-relaxed">
								{form.values.poolSize === 0
									? "With a pool size of 0, runners start on demand. Jobs wait for a workspace to be provisioned before they begin."
									: "Ready runners can pick up jobs immediately. The pool is replenished as runners become busy, within the maximum capacity."}
							</p>
						</div>
						<FormFooter>
							<Button asChild variant="outline">
								<Link to={back}>Cancel</Link>
							</Button>
							<Button type="submit">
								{runner ? "Save changes" : "Create runner"}
							</Button>
						</FormFooter>
					</FormFields>
				</FormSection>
			</HorizontalForm>
		</Margins>
	);
};

export default RunnerFormPage;
