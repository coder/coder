import {
	type FieldSchema,
	getGeneralFields,
	getProviderFields,
	getProviderNames,
	resolveProvider,
	snakeToCamel,
} from "api/chatModelOptions";
import type * as TypesGen from "api/typesGenerated";
import * as Yup from "yup";

// ── Preserved public types ─────────────────────────────────────

export type ModelConfigFormState = Record<
	string,
	string | Record<string, unknown>
>;

export type ModelConfigFormBuildResult = {
	modelConfig?: TypesGen.ChatModelCallConfig;
	fieldErrors: Record<string, string>;
};

export type ModelFormValues = {
	model: string;
	displayName: string;
	contextLimit: string;
	compressionThreshold: string;
	isDefault: boolean;
	config: ModelConfigFormState;
};

// ── Preserved parsing utilities ────────────────────────────────

export const parsePositiveInteger = (value: string): number | null => {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const parsed = Number.parseInt(trimmed, 10);
	if (!Number.isFinite(parsed) || parsed <= 0) return null;
	return parsed;
};

export const parseThresholdInteger = (value: string): number | null => {
	const trimmed = value.trim();
	if (!trimmed) return null;
	const parsed = Number.parseInt(trimmed, 10);
	if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) return null;
	return parsed;
};

// ── Internal helpers ───────────────────────────────────────────

/**
 * Set a value inside a nested object, creating intermediate
 * objects along the way. The path is an array of string keys.
 */
function deepSet(
	obj: Record<string, unknown>,
	path: string[],
	value: unknown,
): void {
	let current = obj;
	for (let i = 0; i < path.length - 1; i++) {
		const key = path[i];
		if (
			current[key] === undefined ||
			current[key] === null ||
			typeof current[key] !== "object"
		) {
			current[key] = {};
		}
		current = current[key] as Record<string, unknown>;
	}
	current[path[path.length - 1]] = value;
}

/**
 * Get a value from a nested object using an array of string keys.
 * Returns `undefined` if any intermediate key is missing.
 */
function deepGet(obj: unknown, path: string[]): unknown {
	let current = obj;
	for (const key of path) {
		if (
			current === undefined ||
			current === null ||
			typeof current !== "object"
		) {
			return undefined;
		}
		current = (current as Record<string, unknown>)[key];
	}
	return current;
}

const hasObjectKeys = (value: Record<string, unknown>): boolean =>
	Object.keys(value).length > 0;

/**
 * Convert a form string value to its API representation based on
 * the field schema type. Empty strings yield `undefined` so
 * callers can conditionally include fields.
 */
function convertFormValue(value: string, field: FieldSchema): unknown {
	const trimmed = value.trim();
	if (!trimmed) return undefined;

	switch (field.type) {
		case "integer":
			return Number.parseInt(trimmed, 10);
		case "number":
			return Number(trimmed);
		case "boolean":
			return trimmed === "true";
		case "array":
		case "object":
			return JSON.parse(trimmed);
		default:
			return trimmed;
	}
}

/**
 * Convert an API value to a form string. Arrays and objects are
 * pretty-printed as JSON; null/undefined become empty strings.
 */
function toFormString(v: unknown): string {
	if (v === undefined || v === null) return "";
	if (typeof v === "object") return JSON.stringify(v, null, 2);
	return String(v);
}

// ── Schema-driven form state generation ────────────────────────

/**
 * Build an empty form state object for a single provider by
 * walking its field schemas. Nested `json_name` values
 * (e.g. `"thinking.budget_tokens"`) produce nested objects with
 * camelCase keys matching `toFormFieldKey`.
 */
function buildEmptyProviderState(provider: string): Record<string, unknown> {
	const state: Record<string, unknown> = {};
	for (const field of getProviderFields(provider)) {
		const camelSegments = field.json_name.split(".").map(snakeToCamel);
		deepSet(state, camelSegments, "");
	}
	return state;
}

/**
 * The empty form state, generated from the schema at module load.
 * Contains general fields as top-level camelCase keys and one
 * nested object per canonical provider.
 */
export const emptyModelConfigFormState: ModelConfigFormState = (() => {
	const state: ModelConfigFormState = {};

	// General fields (e.g. maxOutputTokens, temperature).
	for (const field of getGeneralFields()) {
		const key = snakeToCamel(field.json_name);
		state[key] = "";
	}

	// Provider sub-objects.
	for (const provider of getProviderNames()) {
		state[provider] = buildEmptyProviderState(provider);
	}

	return state;
})();

// ── Extract form state from an existing API model ──────────────

export const extractModelConfigFormState = (
	model: TypesGen.ChatModelConfig,
): ModelConfigFormState => {
	const config = model.model_config;
	if (!config) {
		return structuredClone(emptyModelConfigFormState);
	}

	const state: ModelConfigFormState = {};

	// General fields — read from the top level of the API config
	// using the snake_case json_name.
	for (const field of getGeneralFields()) {
		const camelKey = snakeToCamel(field.json_name);
		const apiValue = (config as Record<string, unknown>)[field.json_name];
		state[camelKey] = toFormString(apiValue);
	}

	// Provider sub-objects.
	const po = config.provider_options;
	for (const provider of getProviderNames()) {
		const providerData = (po as Record<string, unknown> | undefined)?.[
			provider
		] as Record<string, unknown> | undefined;
		const providerState: Record<string, unknown> = {};

		for (const field of getProviderFields(provider)) {
			// Navigate into the API response using snake_case segments.
			const snakeSegments = field.json_name.split(".");
			const apiValue = providerData
				? deepGet(providerData, snakeSegments)
				: undefined;

			// Store under camelCase segments to match the form field key
			// structure produced by toFormFieldKey.
			const camelSegments = field.json_name.split(".").map(snakeToCamel);
			deepSet(providerState, camelSegments, toFormString(apiValue));
		}

		state[provider] = providerState;
	}

	return state;
};

// ── Build initial model form values ────────────────────────────

export const buildInitialModelFormValues = (
	editingModel?: TypesGen.ChatModelConfig,
): ModelFormValues => ({
	model: editingModel?.model ?? "",
	displayName: editingModel?.display_name ?? "",
	contextLimit: editingModel ? String(editingModel.context_limit) : "",
	compressionThreshold: editingModel
		? String(editingModel.compression_threshold)
		: "",
	isDefault: editingModel?.is_default ?? false,
	config: editingModel
		? extractModelConfigFormState(editingModel)
		: structuredClone(emptyModelConfigFormState),
});

// ── Schema-driven Yup validation ───────────────────────────────

/**
 * Build a Yup string test for a single field schema. All form
 * values are strings; the test checks whether a non-empty value
 * can be parsed according to the field's declared type and enum.
 */
function yupTestForField(field: FieldSchema): Yup.StringSchema {
	const label = field.description ?? field.go_name;

	switch (field.type) {
		case "integer":
			return Yup.string().test(
				"optional-integer",
				`${label} must be a valid integer.`,
				(value) => {
					const trimmed = value?.trim();
					if (!trimmed) return true;
					return Number.isFinite(Number.parseInt(trimmed, 10));
				},
			);

		case "number":
			return Yup.string().test(
				"optional-number",
				`${label} must be a valid number.`,
				(value) => {
					const trimmed = value?.trim();
					if (!trimmed) return true;
					return Number.isFinite(Number(trimmed));
				},
			);

		case "boolean":
			return Yup.string().test(
				"optional-boolean",
				`${label} must be true or false.`,
				(value) => {
					const trimmed = value?.trim();
					if (!trimmed) return true;
					return trimmed === "true" || trimmed === "false";
				},
			);

		case "string":
			if (field.enum && field.enum.length > 0) {
				const allowed = field.enum;
				return Yup.string().test(
					"optional-select",
					`${label} has an invalid value.`,
					(value) => {
						const trimmed = value?.trim();
						if (!trimmed) return true;
						return allowed.includes(trimmed);
					},
				);
			}
			// Plain strings are always valid.
			return Yup.string();

		case "array":
			return Yup.string().test(
				"optional-json-array",
				"",
				function validate(value) {
					const trimmed = value?.trim();
					if (!trimmed) return true;
					let parsed: unknown;
					try {
						parsed = JSON.parse(trimmed);
					} catch {
						return this.createError({
							message: `${label} must be valid JSON.`,
						});
					}
					if (!Array.isArray(parsed)) {
						return this.createError({
							message: `${label} must be a JSON array.`,
						});
					}
					return true;
				},
			);

		case "object":
			return Yup.string().test(
				"optional-json-object",
				"",
				function validate(value) {
					const trimmed = value?.trim();
					if (!trimmed) return true;
					let parsed: unknown;
					try {
						parsed = JSON.parse(trimmed);
					} catch {
						return this.createError({
							message: `${label} must be valid JSON.`,
						});
					}
					if (
						typeof parsed !== "object" ||
						parsed === null ||
						Array.isArray(parsed)
					) {
						return this.createError({
							message: `${label} must be a JSON object.`,
						});
					}
					return true;
				},
			);
	}
}

/**
 * Build a Yup object schema for a list of field schemas. Each
 * field is keyed by the **leaf** camelCase name of its json_name.
 * For nested fields (e.g. `thinking.budget_tokens`) we build a
 * nested Yup object shape.
 */
function buildYupSchema(
	fields: FieldSchema[],
): Yup.ObjectSchema<Record<string, unknown>> {
	const shape: Record<string, Yup.AnySchema> = {};

	// Group fields by their first camelCase segment so we can
	// build nested Yup.object() shapes for dot-notation names.
	const nested = new Map<string, FieldSchema[]>();

	for (const field of fields) {
		const segments = field.json_name.split(".");
		if (segments.length === 1) {
			// Top-level field.
			shape[snakeToCamel(segments[0])] = yupTestForField(field);
		} else {
			// Nested — group by first segment.
			const parentKey = snakeToCamel(segments[0]);
			if (!nested.has(parentKey)) {
				nested.set(parentKey, []);
			}
			nested.get(parentKey)!.push(field);
		}
	}

	// Build nested object schemas.
	for (const [parentKey, nestedFields] of nested) {
		const nestedShape: Record<string, Yup.AnySchema> = {};
		for (const field of nestedFields) {
			const segments = field.json_name.split(".");
			// Use the tail segments (after the first) as the nested key.
			const leafKey = segments.slice(1).map(snakeToCamel).join(".");
			nestedShape[leafKey] = yupTestForField(field);
		}
		shape[parentKey] = Yup.object(nestedShape);
	}

	return Yup.object(shape) as Yup.ObjectSchema<Record<string, unknown>>;
}

// Pre-built general-fields schema.
const generalFieldsSchema = buildYupSchema(getGeneralFields());

// Cache of per-provider Yup schemas, built lazily.
const providerSchemaCache = new Map<
	string,
	Yup.ObjectSchema<Record<string, unknown>>
>();

function getProviderYupSchema(
	provider: string,
): Yup.ObjectSchema<Record<string, unknown>> {
	const resolved = resolveProvider(provider);
	let schema = providerSchemaCache.get(resolved);
	if (!schema) {
		schema = buildYupSchema(getProviderFields(resolved));
		providerSchemaCache.set(resolved, schema);
	}
	return schema;
}

// ── Yup error collection ───────────────────────────────────────

type FieldErrors = Record<string, string>;

function collectYupErrors(
	schema: Yup.ObjectSchema<Record<string, unknown>>,
	data: Record<string, unknown>,
	fieldErrors: FieldErrors,
	prefix?: string,
): void {
	try {
		schema.validateSync(data, { abortEarly: false });
	} catch (err) {
		if (err instanceof Yup.ValidationError) {
			for (const inner of err.inner) {
				const key = prefix ? `${prefix}.${inner.path}` : (inner.path ?? "");
				fieldErrors[key] = inner.message;
			}
		}
	}
}

// ── Form → model config builder ────────────────────────────────

export const buildModelConfigFromForm = (
	provider: string | null | undefined,
	form: ModelConfigFormState,
): ModelConfigFormBuildResult => {
	const fieldErrors: FieldErrors = {};

	// Validate general (top-level) fields.
	collectYupErrors(
		generalFieldsSchema,
		form as Record<string, unknown>,
		fieldErrors,
	);

	// Resolve the canonical provider name through the alias table.
	const resolved = resolveProvider((provider ?? "").trim().toLowerCase());

	// Validate provider-specific fields.
	const providerFormState = form[resolved];
	if (providerFormState && typeof providerFormState === "object") {
		collectYupErrors(
			getProviderYupSchema(resolved),
			providerFormState as Record<string, unknown>,
			fieldErrors,
			resolved,
		);
	}

	if (Object.keys(fieldErrors).length > 0) {
		return { fieldErrors };
	}

	// ── Transform general fields ──────────────────────────────
	const modelConfig: Record<string, unknown> = {};

	for (const field of getGeneralFields()) {
		const formValue = form[snakeToCamel(field.json_name)];
		if (typeof formValue !== "string") continue;
		const converted = convertFormValue(formValue, field);
		if (converted !== undefined) {
			modelConfig[field.json_name] = converted;
		}
	}

	// ── Transform provider-specific fields ────────────────────
	if (providerFormState && typeof providerFormState === "object") {
		const providerPayload: Record<string, unknown> = {};

		for (const field of getProviderFields(resolved)) {
			// Read the form value from the nested camelCase structure.
			const camelSegments = field.json_name.split(".").map(snakeToCamel);
			const formValue = deepGet(providerFormState, camelSegments);

			if (typeof formValue !== "string") continue;
			const converted = convertFormValue(formValue, field);
			if (converted === undefined) continue;

			// Write into the API payload using the snake_case
			// json_name segments.
			const snakeSegments = field.json_name.split(".");
			deepSet(providerPayload, snakeSegments, converted);
		}

		if (hasObjectKeys(providerPayload)) {
			modelConfig.provider_options = { [resolved]: providerPayload };
		}
	}

	if (!hasObjectKeys(modelConfig)) {
		return { fieldErrors: {} };
	}

	return {
		modelConfig: modelConfig as TypesGen.ChatModelCallConfig,
		fieldErrors: {},
	};
};
