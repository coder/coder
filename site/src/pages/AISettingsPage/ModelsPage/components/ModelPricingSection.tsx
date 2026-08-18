import { type ChangeEvent, type FC, useState } from "react";
import type { AIModelPrice, AIModelPriceUpsert } from "#/api/typesGenerated";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	InputGroup,
	InputGroupAddon,
	InputGroupInput,
} from "#/components/InputGroup/InputGroup";
import { Label } from "#/components/Label/Label";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import {
	buildModelPriceUpsert,
	type ModelPricingFormErrors,
	type ModelPricingFormValues,
	modelPricingFormValues,
	validateModelPricing,
} from "./modelPricing";

type ModelPricingSectionProps = {
	provider: string;
	model: string;
	price?: AIModelPrice;
	isLoading: boolean;
	isFetching: boolean;
	loadError: unknown;
	isSaving: boolean;
	saveError: unknown;
	isFeatureAvailable: boolean;
	isProviderSupported: boolean;
	isIdentityDirty: boolean;
	canView: boolean;
	canEdit: boolean;
	onSave: (price: AIModelPriceUpsert) => Promise<void>;
};

type PriceField = {
	name: keyof ModelPricingFormValues;
	label: string;
};

type SubmittedPricing = {
	values: ModelPricingFormValues;
	storedValuesAtSubmit: ModelPricingFormValues;
};

const priceFields: readonly PriceField[] = [
	{ name: "inputPrice", label: "Input tokens" },
	{ name: "outputPrice", label: "Output tokens" },
	{ name: "cacheReadPrice", label: "Cache read tokens" },
	{ name: "cacheWritePrice", label: "Cache write tokens" },
];

const valuesEqual = (
	left: ModelPricingFormValues,
	right: ModelPricingFormValues,
): boolean => priceFields.every(({ name }) => left[name] === right[name]);

export const ModelPricingSection: FC<ModelPricingSectionProps> = ({
	provider,
	model,
	price,
	isLoading,
	isFetching,
	loadError,
	isSaving,
	saveError,
	isFeatureAvailable,
	isProviderSupported,
	isIdentityDirty,
	canView,
	canEdit,
	onSave,
}) => {
	const storedValues = modelPricingFormValues(price);
	const [draft, setDraft] = useState<ModelPricingFormValues | null>(null);
	const [submitted, setSubmitted] = useState<SubmittedPricing | null>(null);
	const [errors, setErrors] = useState<ModelPricingFormErrors>({});
	const submittedValues =
		submitted !== null &&
		valuesEqual(storedValues, submitted.storedValuesAtSubmit)
			? submitted.values
			: null;
	const values = draft ?? submittedValues ?? storedValues;
	const priceBookManaged = price?.is_default === true;
	const editable =
		isFeatureAvailable &&
		isProviderSupported &&
		canView &&
		canEdit &&
		!priceBookManaged &&
		!isIdentityDirty;
	const dirty =
		draft !== null && !valuesEqual(values, submittedValues ?? storedValues);

	const updateField =
		(name: keyof ModelPricingFormValues) =>
		(event: ChangeEvent<HTMLInputElement>) => {
			setDraft({ ...values, [name]: event.target.value });
			setErrors((current) => ({ ...current, [name]: undefined }));
		};

	const save = () => {
		const nextErrors = validateModelPricing(values);
		setErrors(nextErrors);
		if (Object.keys(nextErrors).length > 0) {
			return;
		}
		const request = buildModelPriceUpsert(provider, model, values);
		if (!request) {
			setErrors({
				inputPrice:
					"Set at least one price. Leave individual fields blank when that price is unknown.",
			});
			return;
		}
		const storedValuesAtSubmit = storedValues;
		void onSave(request).then(
			() => {
				setSubmitted({ values, storedValuesAtSubmit });
				setDraft(null);
				setErrors({});
			},
			() => undefined,
		);
	};

	return (
		<section
			aria-labelledby="model-pricing-heading"
			aria-busy={isFetching}
			className="border border-solid p-6 rounded-lg"
		>
			<div className="flex flex-col gap-1">
				<h2
					id="model-pricing-heading"
					className="m-0 text-base font-medium text-content-primary"
				>
					Model pricing
				</h2>
				<p className="m-0 text-sm text-content-secondary">
					Prices in USD per million tokens are used for AI Gateway spend
					reporting and budget enforcement.
				</p>
			</div>

			<div className="mt-5 flex flex-col gap-4">
				{!isFeatureAvailable ? (
					<Alert severity="info">
						<AlertTitle>AI Bridge required</AlertTitle>
						<AlertDescription>
							Model pricing is available with the AI Bridge feature.
						</AlertDescription>
					</Alert>
				) : !isProviderSupported ? (
					<Alert severity="info">
						<AlertTitle>Pricing unavailable</AlertTitle>
						<AlertDescription>
							Model pricing is not supported for OpenAI-compatible providers.
						</AlertDescription>
					</Alert>
				) : isLoading ? (
					<div
						role="status"
						aria-label="Loading model pricing"
						className="grid gap-4 sm:grid-cols-2"
					>
						{priceFields.map(({ name }) => (
							<div key={name} className="flex flex-col gap-2">
								<Skeleton height={14} width={120} />
								<Skeleton height={40} />
							</div>
						))}
					</div>
				) : !canView ? (
					<Alert severity="info">
						<AlertTitle>Pricing unavailable</AlertTitle>
						<AlertDescription>
							You do not have permission to view AI model prices.
						</AlertDescription>
					</Alert>
				) : loadError && !price ? (
					<ErrorAlert error={loadError} showDebugDetail={false} />
				) : (
					<>
						{priceBookManaged && (
							<Alert severity="info">
								<AlertTitle>Managed by Coder&apos;s price book</AlertTitle>
								<AlertDescription>
									These prices ship with this Coder version and cannot be
									edited.
								</AlertDescription>
							</Alert>
						)}
						{!priceBookManaged && !price && (
							<Alert severity="info">
								<AlertTitle>No stored pricing</AlertTitle>
								<AlertDescription>
									Blank fields mean the price is unknown. Set at least one price
									to start tracking spend for this model.
								</AlertDescription>
							</Alert>
						)}
						{!priceBookManaged && isIdentityDirty && (
							<Alert severity="info">
								<AlertTitle>Save model identity changes</AlertTitle>
								<AlertDescription>
									Save the provider or model identifier changes before editing
									pricing.
								</AlertDescription>
							</Alert>
						)}
						{!priceBookManaged && !canEdit && (
							<Alert severity="info">
								<AlertTitle>Read-only pricing</AlertTitle>
								<AlertDescription>
									You do not have permission to update AI model prices.
								</AlertDescription>
							</Alert>
						)}

						<div className="grid gap-4 sm:grid-cols-2">
							{priceFields.map(({ name, label }) => {
								const error = errors[name];
								const errorId = `model-pricing-${name}-error`;
								return (
									<div key={name} className="flex min-w-0 flex-col gap-1.5">
										<Label htmlFor={`model-pricing-${name}`}>{label}</Label>
										<InputGroup
											className={cn(error && "border-border-destructive")}
										>
											<InputGroupAddon align="inline-start">$</InputGroupAddon>
											<InputGroupInput
												id={`model-pricing-${name}`}
												name={name}
												inputMode="decimal"
												placeholder="Unknown"
												value={values[name]}
												onChange={updateField(name)}
												disabled={!editable || isSaving}
												aria-invalid={Boolean(error)}
												aria-describedby={error ? errorId : undefined}
											/>
											<InputGroupAddon align="inline-end">
												<span className="text-xs text-content-disabled">
													USD/1M tokens
												</span>
											</InputGroupAddon>
										</InputGroup>
										{error && (
											<p
												id={errorId}
												className="m-0 text-xs text-content-destructive"
											>
												{error}
											</p>
										)}
									</div>
								);
							})}
						</div>

						{loadError && price && (
							<ErrorAlert error={loadError} showDebugDetail={false} />
						)}
						{saveError && (
							<ErrorAlert error={saveError} showDebugDetail={false} />
						)}
						{isFetching && !isLoading && (
							<p role="status" className="m-0 text-xs text-content-secondary">
								Refreshing model pricing.
							</p>
						)}
						{editable && (
							<div className="flex justify-end">
								<Button
									type="button"
									disabled={!dirty || isSaving}
									onClick={save}
								>
									{isSaving && <Spinner loading />}
									Save pricing
								</Button>
							</div>
						)}
					</>
				)}
			</div>
		</section>
	);
};
