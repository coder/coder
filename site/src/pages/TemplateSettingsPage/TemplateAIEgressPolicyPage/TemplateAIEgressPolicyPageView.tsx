import { PlusIcon, Trash2Icon } from "lucide-react";
import { type FC, type FormEvent, useState } from "react";
import type {
	AIEgressPolicy,
	AIEgressRule,
	UpdateAIEgressPolicyRequest,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "#/components/Form/Form";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";

const MAX_RULES = 128;

type RuleDraft = {
	host: string;
	ports: string;
};

type RuleValidation = {
	host?: string;
	ports?: string;
};

interface TemplateAIEgressPolicyPageViewProps {
	policy: AIEgressPolicy;
	canUpdate: boolean;
	isFetching: boolean;
	isSubmitting: boolean;
	loadError?: unknown;
	submitError?: unknown;
	onSubmit: (
		request: UpdateAIEgressPolicyRequest,
		onSuccess: (policy: AIEgressPolicy) => void,
	) => void;
}

const rulesToDraft = (rules: readonly AIEgressRule[]): RuleDraft[] =>
	rules.map((rule) => ({
		host: rule.host,
		ports: rule.ports?.join(", ") ?? "",
	}));

const normalizeRules = (rules: readonly AIEgressRule[]): AIEgressRule[] =>
	rules.map((rule) => ({
		host: rule.host.toLowerCase(),
		ports: [...(rule.ports ?? [])],
	}));

const validateHost = (host: string): string | undefined => {
	if (host.trim() === "") {
		return "Host is required.";
	}
	if (host.length > 253) {
		return "Host must be no more than 253 characters.";
	}
	if (/\s/.test(host)) {
		return "Host must not contain whitespace.";
	}
	if (/[/@:]/.test(host)) {
		return "Host must not contain a scheme, path, port, or user information.";
	}
	if (
		host.includes("*") &&
		(!host.startsWith("*.") ||
			host.split("*").length !== 2 ||
			host.length === 2 ||
			host.slice(2).startsWith("."))
	) {
		return "Wildcard must be a single leading '*.' label.";
	}
	return undefined;
};

const parsePorts = (value: string): { ports: number[]; error?: string } => {
	if (value.trim() === "") {
		return { ports: [] };
	}

	const entries = value.split(",").map((entry) => entry.trim());
	if (entries.some((entry) => entry === "" || !/^\d+$/.test(entry))) {
		return {
			ports: [],
			error: "Ports must be comma-separated integers between 1 and 65535.",
		};
	}

	const ports = entries.map(Number);
	if (ports.some((port) => port < 1 || port > 65535)) {
		return {
			ports: [],
			error: "Ports must be between 1 and 65535.",
		};
	}

	return { ports: [...new Set(ports)] };
};

const validateRules = (rules: readonly RuleDraft[]): RuleValidation[] =>
	rules.map((rule) => ({
		host: validateHost(rule.host),
		ports: parsePorts(rule.ports).error,
	}));

const buildRequest = (
	rules: readonly RuleDraft[],
): UpdateAIEgressPolicyRequest => ({
	rules: rules.map((rule) => ({
		host: rule.host.toLowerCase(),
		ports: parsePorts(rule.ports).ports,
	})),
});

export const TemplateAIEgressPolicyPageView: FC<
	TemplateAIEgressPolicyPageViewProps
> = ({
	policy,
	canUpdate,
	isFetching,
	isSubmitting,
	loadError,
	submitError,
	onSubmit,
}) => {
	const [baselinePolicy, setBaselinePolicy] = useState(policy);
	const [rules, setRules] = useState<RuleDraft[]>(() =>
		rulesToDraft(policy.rules),
	);
	const validations = validateRules(rules);
	const hasValidationErrors = validations.some(
		(validation) => validation.host || validation.ports,
	);
	const request = buildRequest(rules);
	const isDirty =
		JSON.stringify(request.rules) !==
		JSON.stringify(normalizeRules(baselinePolicy.rules));
	const readOnlyDescriptionId = "ai-egress-policy-read-only";
	const controlsDescription = canUpdate ? undefined : readOnlyDescriptionId;
	const isAtRuleLimit = rules.length >= MAX_RULES;

	const updateRule = (index: number, changes: Partial<RuleDraft>) => {
		setRules((currentRules) =>
			currentRules.map((rule, currentIndex) =>
				currentIndex === index ? { ...rule, ...changes } : rule,
			),
		);
	};

	const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		if (
			!canUpdate ||
			isSubmitting ||
			hasValidationErrors ||
			!isDirty ||
			rules.length > MAX_RULES
		) {
			return;
		}

		onSubmit(request, (savedPolicy) => {
			setBaselinePolicy(savedPolicy);
			setRules(rulesToDraft(savedPolicy.rules));
		});
	};

	return (
		<div className="flex flex-col gap-10">
			<SettingsHeader>
				<SettingsHeaderTitle>AI Egress Policy</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					This policy is default-deny beyond implicit control-plane rules. The
					Coder access URL and configured AI gateway are always allowed and
					cannot be edited here.
				</SettingsHeaderDescription>
			</SettingsHeader>

			<div className="flex flex-col gap-4">
				<div className="flex min-h-6 items-center gap-3 text-sm text-content-secondary">
					<strong className="text-content-primary">
						Revision {policy.revision}
					</strong>
					{isFetching && (
						<span role="status" className="flex items-center gap-2">
							<Spinner size="sm" loading />
							Refreshing policy
						</span>
					)}
				</div>
				<Alert severity="info">
					Changes create a new policy revision and are pushed live to running
					supervisors. No template version push or workspace rebuild is
					required.
				</Alert>
				{!canUpdate && (
					<Alert severity="info">
						<span id={readOnlyDescriptionId}>
							You can inspect this policy, but you do not have permission to
							update this template.
						</span>
					</Alert>
				)}
				{Boolean(loadError) && <ErrorAlert error={loadError} />}
				{Boolean(submitError) && <ErrorAlert error={submitError} />}
			</div>

			<HorizontalForm
				aria-label="AI egress policy form"
				onSubmit={handleSubmit}
			>
				<FormSection
					title="Allowlist rules"
					description={
						<>
							Hosts contain no scheme, path, or port. Configure ports
							separately. A wildcard is one leading label only. For example,{" "}
							<code>*.example.com</code> matches <code>api.example.com</code>,
							but not <code>example.com</code> or <code>a.b.example.com</code>.
							Empty ports allow 80 and 443.
						</>
					}
				>
					<FormFields>
						{rules.length === 0 && (
							<Alert severity="info">
								No explicit egress rules are configured. Only implicit
								control-plane destinations are allowed.
							</Alert>
						)}

						<fieldset
							disabled={!canUpdate || isSubmitting}
							aria-describedby={controlsDescription}
							className="m-0 flex min-w-0 flex-col gap-4 border-0 p-0"
						>
							<legend className="sr-only">AI egress allowlist rules</legend>
							{rules.map((rule, index) => {
								const hostId = `ai-egress-rule-${index}-host`;
								const hostErrorId = `${hostId}-error`;
								const portsId = `ai-egress-rule-${index}-ports`;
								const portsErrorId = `${portsId}-error`;
								const validation = validations[index];

								return (
									<fieldset
										key={index}
										className="m-0 min-w-0 rounded-lg border border-border-default p-4"
									>
										<legend className="px-2 text-sm font-medium">
											Rule {index + 1}
										</legend>
										<div className="grid gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,0.7fr)_auto] md:items-start">
											<div className="flex min-w-0 flex-col gap-2">
												<Label htmlFor={hostId}>Host</Label>
												<Input
													id={hostId}
													value={rule.host}
													onChange={(event) =>
														updateRule(index, { host: event.target.value })
													}
													maxLength={253}
													placeholder="example.com"
													required
													aria-invalid={Boolean(validation?.host)}
													aria-describedby={
														validation?.host ? hostErrorId : undefined
													}
												/>
												{validation?.host && (
													<span
														id={hostErrorId}
														className="text-xs text-content-destructive"
													>
														{validation.host}
													</span>
												)}
											</div>

											<div className="flex min-w-0 flex-col gap-2">
												<Label htmlFor={portsId}>Ports</Label>
												<Input
													id={portsId}
													value={rule.ports}
													onChange={(event) =>
														updateRule(index, { ports: event.target.value })
													}
													placeholder="80, 443"
													inputMode="numeric"
													aria-invalid={Boolean(validation?.ports)}
													aria-describedby={
														validation?.ports ? portsErrorId : undefined
													}
												/>
												{validation?.ports && (
													<span
														id={portsErrorId}
														className="text-xs text-content-destructive"
													>
														{validation.ports}
													</span>
												)}
											</div>

											<Button
												variant="subtle"
												size="sm"
												className="md:mt-7"
												onClick={() =>
													setRules((currentRules) =>
														currentRules.filter(
															(_, currentIndex) => currentIndex !== index,
														),
													)
												}
											>
												<Trash2Icon />
												Remove rule {index + 1}
											</Button>
										</div>
									</fieldset>
								);
							})}

							<Button
								variant="outline"
								className="self-start"
								onClick={() =>
									setRules((currentRules) => [
										...currentRules,
										{ host: "", ports: "" },
									])
								}
								disabled={isAtRuleLimit}
							>
								<PlusIcon />
								Add rule
							</Button>
							{isAtRuleLimit && (
								<p className="m-0 text-xs text-content-secondary">
									A policy can contain at most {MAX_RULES} rules.
								</p>
							)}
						</fieldset>
					</FormFields>
				</FormSection>

				<FormFooter>
					<Button
						type="submit"
						disabled={
							!canUpdate ||
							isSubmitting ||
							hasValidationErrors ||
							!isDirty ||
							rules.length > MAX_RULES
						}
						aria-describedby={controlsDescription}
					>
						{isSubmitting ? (
							<>
								<Spinner size="sm" loading />
								Saving policy
							</>
						) : (
							"Save policy"
						)}
					</Button>
				</FormFooter>
			</HorizontalForm>
		</div>
	);
};
