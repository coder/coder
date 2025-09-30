import { css } from "@emotion/css";
import MenuItem from "@mui/material/MenuItem";
import TextField from "@mui/material/TextField";
import type { APIAllowListTarget, ScopeCatalog } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import {
	FormFields,
	FormFooter,
	FormSection,
	HorizontalForm,
} from "components/Form/Form";
import {
	MultiSelectCombobox,
	type MultiSelectComboboxRef,
	type Option,
} from "components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";
import type { FormikContextType } from "formik";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useNavigate } from "react-router";
import { getFormHelpers, onChangeTrimmed } from "utils/formUtils";
import {
	allowListTargetsToStrings,
	buildCompositeExpansionMap,
	type CreateTokenData,
	customLifetimeDay,
	determineDefaultLtValue,
	expandCompositeScopes,
	filterByMaxTokenLifetime,
	NANO_HOUR,
	type ScopeSelectionMode,
	sortScopes,
} from "./utils";

dayjs.extend(utc);

interface CreateTokenFormProps {
	form: FormikContextType<CreateTokenData>;
	maxTokenLifetime?: number;
	formError: unknown;
	setFormError: (arg0: unknown) => void;
	isSubmitting: boolean;
	submitFailed: boolean;
	scopeCatalog?: ScopeCatalog;
	initialAllowListTargets?: readonly APIAllowListTarget[];
	resolveAllowListOptions: (query: string) => Promise<Option[]>;
	submitLabel?: string;
	nameDisabled?: boolean;
}

interface ParsedAllowListValue {
	normalizedType: string;
	type: string;
	id: string;
}

const parseAllowListValue = (rawValue: string): ParsedAllowListValue => {
	const trimmed = rawValue.trim();
	if (trimmed === "") {
		return { normalizedType: "", type: "", id: "" };
	}
	const [typePart = "", idPart = ""] = trimmed.split(":", 2);
	const type = typePart.trim() || "*";
	const id = idPart.trim() || "*";
	return {
		normalizedType: type === "*" ? "*" : type.toLowerCase(),
		type,
		id,
	};
};

const formatAllowListOptionLabel = (value: string, label: string): string => {
	const parsed = parseAllowListValue(value);
	if (parsed.type === "" || parsed.type === "*") {
		return label;
	}
	const trimmedLabel = label.trim();
	const lowercaseLabel = trimmedLabel.toLowerCase();
	const prefix = `${parsed.type} :`;
	if (lowercaseLabel.startsWith(prefix.toLowerCase())) {
		return trimmedLabel;
	}
	return `${parsed.type} : ${trimmedLabel}`;
};

const pluralizeResourceType = (type: string): string => {
	if (type === "" || type === "*") {
		return type;
	}
	return type.endsWith("s") ? type : `${type}s`;
};

const buildFallbackAllowListOption = (
	value: string,
	parsed: ParsedAllowListValue,
): Option => {
	if (parsed.type === "*" && parsed.id === "*") {
		return { value, label: "Any resource", group: "Wildcard" };
	}
	if (parsed.id === "*") {
		if (parsed.normalizedType === "*" || parsed.normalizedType === "") {
			return {
				value,
				label: formatAllowListOptionLabel(value, value),
				group: "wildcard",
			};
		}
		const typeForLabel = parsed.normalizedType;
		const plural = pluralizeResourceType(typeForLabel);
		return {
			value,
			label: formatAllowListOptionLabel(value, `All ${plural}`),
			group: plural,
		};
	}
	const group = parsed.normalizedType
		? pluralizeResourceType(parsed.normalizedType)
		: undefined;
	return {
		value,
		label: formatAllowListOptionLabel(value, parsed.id),
		group,
	};
};

export const CreateTokenForm: FC<CreateTokenFormProps> = ({
	form,
	maxTokenLifetime,
	formError,
	setFormError,
	isSubmitting,
	submitFailed,
	scopeCatalog,
	initialAllowListTargets,
	resolveAllowListOptions,
	submitLabel = "Create token",
	nameDisabled = false,
}) => {
	const navigate = useNavigate();
	const allowListComboboxRef = useRef<MultiSelectComboboxRef>(null);

	const [expDays, setExpDays] = useState<number>(1);
	const [lifetimeDays, setLifetimeDays] = useState<number | string>(
		determineDefaultLtValue(maxTokenLifetime),
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies: adding form will cause an infinite loop
	useEffect(() => {
		if (lifetimeDays !== "custom") {
			void form.setFieldValue("lifetime", lifetimeDays);
		} else {
			void form.setFieldValue("lifetime", expDays);
		}
	}, [lifetimeDays, expDays]);

	const getFieldHelpers = getFormHelpers<CreateTokenData>(form, formError);
	const scopeMode = form.values.scopeMode;

	const compositeExpansionMap = useMemo(() => {
		return buildCompositeExpansionMap(scopeCatalog);
	}, [scopeCatalog]);

	const autoLowLevelScopes = useMemo(() => {
		return expandCompositeScopes(
			form.values.compositeScopes,
			compositeExpansionMap,
		);
	}, [form.values.compositeScopes, compositeExpansionMap]);

	const effectiveLowLevelScopes = useMemo(() => {
		return sortScopes([
			...new Set([...autoLowLevelScopes, ...form.values.lowLevelScopes]),
		]);
	}, [autoLowLevelScopes, form.values.lowLevelScopes]);

	const effectiveCompositeScopes = useMemo(() => {
		return sortScopes(form.values.compositeScopes);
	}, [form.values.compositeScopes]);

	const lowLevelOptions = useMemo(() => {
		if (!scopeCatalog) {
			return [] as Option[];
		}
		return scopeCatalog.low_level.map((item) => ({
			value: item.name,
			label: formatLowLevelLabel(item.name),
			group: item.resource,
		}));
	}, [scopeCatalog]);

	const lowLevelOptionMap = useMemo(() => {
		const map = new Map<string, Option>();
		for (const option of lowLevelOptions) {
			map.set(option.value, option);
		}
		return map;
	}, [lowLevelOptions]);

	const selectedLowLevelOptions = useMemo(() => {
		return form.values.lowLevelScopes
			.map((value) => lowLevelOptionMap.get(value))
			.filter((option): option is Option => Boolean(option));
	}, [form.values.lowLevelScopes, lowLevelOptionMap]);

	const [hydratedAllowList, setHydratedAllowList] = useState<
		Record<string, Option>
	>({});
	const hydrationAttemptsRef = useRef(new Set<string>());
	const previousInitialAllowListSignatureRef = useRef<string | null>(null);

	useEffect(() => {
		if (!initialAllowListTargets) {
			previousInitialAllowListSignatureRef.current = null;
			return;
		}
		const normalizedInitial = allowListTargetsToStrings(initialAllowListTargets)
			.map((entry) => entry.trim())
			.filter((entry): entry is string => entry !== "");
		const signature =
			normalizedInitial.length === 0 ? "" : normalizedInitial.join("|");
		if (signature === previousInitialAllowListSignatureRef.current) {
			return;
		}
		previousInitialAllowListSignatureRef.current = signature;
		if (normalizedInitial.length === 0) {
			return;
		}
		void form.setFieldValue("allowList", normalizedInitial);
	}, [form.setFieldValue, initialAllowListTargets]);

	useEffect(() => {
		if (!initialAllowListTargets || initialAllowListTargets.length === 0) {
			return;
		}
		const updates: Record<string, Option> = {};
		initialAllowListTargets.forEach((target) => {
			const displayName = target.display_name?.trim();
			if (!displayName) {
				return;
			}
			const value = `${target.type}:${target.id}`;
			const parsed = parseAllowListValue(value);
			const baseOption = buildFallbackAllowListOption(value, parsed);
			updates[value] = {
				...baseOption,
				value,
				label: formatAllowListOptionLabel(value, displayName),
			};
		});
		if (Object.keys(updates).length === 0) {
			return;
		}
		setHydratedAllowList((prev) => {
			let changed = false;
			const next = { ...prev };
			Object.entries(updates).forEach(([key, option]) => {
				const existing = next[key];
				if (
					!existing ||
					existing.label !== option.label ||
					existing.description !== option.description
				) {
					next[key] = option;
					changed = true;
				}
			});
			return changed ? next : prev;
		});
	}, [initialAllowListTargets]);

	const allowListValueOptions = useMemo(() => {
		return form.values.allowList
			.map((rawValue) => {
				const value = rawValue.trim();
				if (value === "") {
					return undefined;
				}
				const parsed = parseAllowListValue(value);
				const hydrated = hydratedAllowList[value];
				if (hydrated) {
					return {
						...hydrated,
						value,
						label: formatAllowListOptionLabel(value, hydrated.label),
					};
				}
				return buildFallbackAllowListOption(value, parsed);
			})
			.filter((option): option is Option => Boolean(option));
	}, [form.values.allowList, hydratedAllowList]);

	const handleAllowListSearch = useCallback(
		(query: string) => resolveAllowListOptions(query),
		[resolveAllowListOptions],
	);

	const handleScopeModeChange = (mode: ScopeSelectionMode) => {
		if (mode === scopeMode) {
			return;
		}
		void form.setFieldValue("scopeMode", mode);
	};

	const handleCompositeToggle = (scope: string) => {
		const current = new Set(form.values.compositeScopes);
		if (current.has(scope)) {
			current.delete(scope);
		} else {
			current.add(scope);
		}
		void form.setFieldValue("compositeScopes", Array.from(current));
	};

	const handleLowLevelChange = (options: Option[]) => {
		void form.setFieldValue(
			"lowLevelScopes",
			options.map((option) => option.value),
		);
	};

	const handleAllowListChange = useCallback(
		(options: Option[]) => {
			const normalized: Option[] = [];
			let pendingPrefix: string | undefined;

			for (const option of options) {
				const prefix = (option as { prefix?: string }).prefix;
				if (prefix) {
					pendingPrefix = prefix;
					continue;
				}
				const trimmedValue = option.value.trim();
				if (trimmedValue === "") {
					continue;
				}
				const normalizedOption: Option = {
					...option,
					value: trimmedValue,
					label: formatAllowListOptionLabel(trimmedValue, option.label),
				};
				normalized.push(normalizedOption);
			}

			if (normalized.length > 0) {
				setHydratedAllowList((prev) => {
					let changed = false;
					const next = { ...prev };
					for (const option of normalized) {
						const existing = next[option.value];
						if (
							!existing ||
							existing.label !== option.label ||
							existing.description !== option.description
						) {
							next[option.value] = option;
							changed = true;
						}
					}
					return changed ? next : prev;
				});
			}

			void form.setFieldValue(
				"allowList",
				normalized.map((option) => option.value),
			);

			if (pendingPrefix) {
				allowListComboboxRef.current?.setInputValue(pendingPrefix, {
					focus: true,
					open: true,
				});
			}
		},
		[form],
	);

	useEffect(() => {
		if (form.values.allowList.length === 0) {
			hydrationAttemptsRef.current.clear();
			return;
		}

		const currentValues = new Set(
			form.values.allowList.map((raw) => raw.trim()).filter(Boolean),
		);
		for (const value of Array.from(hydrationAttemptsRef.current)) {
			if (!currentValues.has(value)) {
				hydrationAttemptsRef.current.delete(value);
			}
		}

		const outstanding = Array.from(
			new Set(
				form.values.allowList
					.map((raw) => raw.trim())
					.filter((value) => {
						if (value === "") {
							return false;
						}
						const parsed = parseAllowListValue(value);
						if (parsed.id === "*") {
							return false;
						}
						if (hydrationAttemptsRef.current.has(value)) {
							return false;
						}
						const hydrated = hydratedAllowList[value];
						if (!hydrated) {
							return true;
						}
						const friendlyLabel = hydrated.label?.trim();
						return (
							!friendlyLabel ||
							friendlyLabel === parsed.id ||
							friendlyLabel === value
						);
					}),
			),
		);

		if (outstanding.length === 0) {
			return;
		}

		let cancelled = false;

		void (async () => {
			const updates: Record<string, Option> = {};
			for (const target of outstanding) {
				hydrationAttemptsRef.current.add(target);
				try {
					const options = await resolveAllowListOptions(target);
					const match = options.find(
						(option) => option.value.trim() === target,
					);
					if (match) {
						updates[target] = {
							...match,
							value: target,
							label: formatAllowListOptionLabel(target, match.label),
						};
					}
				} catch (error) {
					console.error(`Failed to hydrate allow-list entry ${target}`, error);
				}
			}
			if (!cancelled && Object.keys(updates).length > 0) {
				setHydratedAllowList((prev) => ({
					...prev,
					...updates,
				}));
			}
		})();

		return () => {
			cancelled = true;
		};
	}, [form.values.allowList, hydratedAllowList, resolveAllowListOptions]);

	return (
		<HorizontalForm onSubmit={form.handleSubmit}>
			<FormSection
				title="Name"
				description="What is this token for?"
				classes={{ sectionInfo: classNames.sectionInfo }}
			>
				<FormFields>
					<TextField
						{...getFieldHelpers("name")}
						label="Name"
						required
						onChange={onChangeTrimmed(form, () => setFormError(undefined))}
						autoFocus={!nameDisabled}
						disabled={nameDisabled}
						fullWidth
					/>
				</FormFields>
			</FormSection>
			<FormSection
				title="Expiration"
				description={
					form.values.lifetime ? (
						<>
							The token will expire on{" "}
							<span data-chromatic="ignore">
								{dayjs()
									.add(form.values.lifetime, "days")
									.utc()
									.format("MMMM DD, YYYY")}
							</span>
						</>
					) : (
						"Please set a token expiration."
					)
				}
				classes={{ sectionInfo: classNames.sectionInfo }}
			>
				<FormFields>
					<Stack direction="row">
						<TextField
							select
							label="Lifetime"
							required
							defaultValue={determineDefaultLtValue(maxTokenLifetime)}
							onChange={(event) => {
								void setLifetimeDays(event.target.value);
							}}
							fullWidth
						>
							{filterByMaxTokenLifetime(maxTokenLifetime).map((lt) => (
								<MenuItem key={lt.label} value={lt.value}>
									{lt.label}
								</MenuItem>
							))}
							<MenuItem
								key={customLifetimeDay.label}
								value={customLifetimeDay.value}
							>
								{customLifetimeDay.label}
							</MenuItem>
						</TextField>

						{lifetimeDays === "custom" && (
							<TextField
								type="date"
								label="Expires on"
								defaultValue={dayjs().add(expDays, "day").format("YYYY-MM-DD")}
								onChange={(event) => {
									const lt = Math.ceil(
										dayjs(event.target.value).diff(dayjs(), "day", true),
									);
									setExpDays(lt);
								}}
								inputProps={{
									"data-chromatic": "ignore",
									min: dayjs().add(1, "day").format("YYYY-MM-DD"),
									max: maxTokenLifetime
										? dayjs()
												.add(maxTokenLifetime / NANO_HOUR / 24, "day")
												.format("YYYY-MM-DD")
										: undefined,
									required: true,
								}}
								fullWidth
								InputLabelProps={{
									required: true,
								}}
							/>
						)}
					</Stack>
				</FormFields>
			</FormSection>
			<FormSection
				title="Scopes"
				description="Select the capabilities this token should have. Composite scopes automatically include the low-level scope atoms they require."
				classes={{ sectionInfo: classNames.sectionInfo }}
			>
				<FormFields>
					<Stack spacing={3}>
						<div className="flex gap-2">
							<Button
								type="button"
								variant={scopeMode === "composite" ? "outline" : "subtle"}
								size="sm"
								onClick={() => handleScopeModeChange("composite")}
							>
								Composite scopes
							</Button>
							<Button
								type="button"
								variant={scopeMode === "low_level" ? "outline" : "subtle"}
								size="sm"
								onClick={() => handleScopeModeChange("low_level")}
							>
								Low-level scopes
							</Button>
						</div>

						{scopeMode === "composite" ? (
							<div className="flex flex-col gap-3">
								{(scopeCatalog?.composites ?? []).map((composite) => {
									const checked = form.values.compositeScopes.includes(
										composite.name,
									);
									return (
										<div
											key={composite.name}
											className="flex gap-3 items-start border border-border border-solid rounded-md p-3"
										>
											<Checkbox
												checked={checked}
												onCheckedChange={() =>
													handleCompositeToggle(composite.name)
												}
											/>
											<div>
												<div className="font-medium">
													{formatCompositeLabel(composite.name)}
												</div>
												<div className="text-sm text-content-secondary">
													Includes{" "}
													{formatCompositeExpansions(
														composite.expands_to ?? [],
													)}
												</div>
											</div>
										</div>
									);
								})}
							</div>
						) : (
							<MultiSelectCombobox
								value={selectedLowLevelOptions}
								options={lowLevelOptions}
								groupBy="group"
								placeholder="Search low-level scopes"
								onSearchSync={(needle) =>
									lowLevelOptions.filter((option) =>
										option.label.toLowerCase().includes(needle.toLowerCase()),
									)
								}
								onChange={handleLowLevelChange}
								creatable={false}
							/>
						)}

						<div className="rounded-md bg-surface-secondary px-3 py-2 text-sm text-content-secondary">
							<div className="font-medium text-content-primary">
								Selected scopes preview
							</div>
							<div className="mt-1">
								<strong>Composite:</strong>{" "}
								{effectiveCompositeScopes.length === 0
									? "None"
									: effectiveCompositeScopes
											.map(formatCompositeLabel)
											.join(", ")}
							</div>
							<div className="mt-1">
								<strong>Low-level:</strong>{" "}
								{effectiveLowLevelScopes.length === 0
									? "None"
									: effectiveLowLevelScopes.map(formatLowLevelLabel).join(", ")}
							</div>
						</div>
					</Stack>
				</FormFields>
			</FormSection>
			<FormSection
				title="Allow-list"
				description="Optionally restrict the token to specific resources. Leave empty to allow all resources."
				classes={{ sectionInfo: classNames.sectionInfo }}
			>
				<FormFields>
					<MultiSelectCombobox
						ref={allowListComboboxRef}
						value={allowListValueOptions}
						onChange={handleAllowListChange}
						onSearch={handleAllowListSearch}
						placeholder="Type a resource prefix, e.g. workspace: or user:"
						delay={200}
						groupBy="group"
						triggerSearchOnFocus
					/>
					<p className="text-xs text-content-secondary mt-1">
						Examples: <code>workspace:All workspaces</code>,{" "}
						<code>user:All users</code>, <code>template:my-template</code>,{" "}
						<code>Any resource</code>
					</p>
				</FormFields>
			</FormSection>

			<FormFooter>
				<Button onClick={() => navigate("/settings/tokens")} variant="outline">
					Cancel
				</Button>
				<Button type="submit" disabled={isSubmitting}>
					<Spinner loading={isSubmitting} />
					{submitFailed ? "Retry" : submitLabel}
				</Button>
			</FormFooter>
		</HorizontalForm>
	);
};

const classNames = {
	sectionInfo: css`
    min-width: 300px;
  `,
};

const titleCase = (value: string) => {
	if (value === "*" || value === "") {
		return "Any";
	}
	return value
		.split(/[^a-zA-Z0-9]+/)
		.filter(Boolean)
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(" ");
};

const formatCompositeLabel = (scope: string) => {
	const [, raw = scope] = scope.split(":", 2);
	return titleCase(raw.replaceAll(".", " "));
};

const formatLowLevelLabel = (scope: string) => {
	const [resource = scope, action = "*"] = scope.split(":", 2);
	return `${titleCase(resource)} · ${titleCase(action)}`;
};

const formatCompositeExpansions = (scopes: readonly string[]) => {
	if (!scopes || scopes.length === 0) {
		return "no additional low-level scopes";
	}
	return scopes.map(formatLowLevelLabel).join(", ");
};
