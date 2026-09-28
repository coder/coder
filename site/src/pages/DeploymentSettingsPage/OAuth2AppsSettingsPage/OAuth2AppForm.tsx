import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink } from "react-router";
import * as Yup from "yup";
import { getErrorMessage } from "#/api/errors";
import { getExternalScopes } from "#/api/queries/oauth2";
import type * as TypesGen from "#/api/typesGenerated";
import {
	OAuth2AppNameMaxBytes,
	OAuth2RedirectURIMaxBytes,
	OAuth2RedirectURIsMaxCount,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { MultiSelectCombobox } from "#/components/MultiSelectCombobox/MultiSelectCombobox";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { docs } from "#/utils/docs";
import { getFormHelpers, iconValidator } from "#/utils/formUtils";
import { RedirectURIsField } from "./components/RedirectURIsField";

type OAuth2AppFormValues = {
	name: string;
	redirect_uris: string[];
	icon: string;
	scope: string[];
};

// A create always sends the redirect URI list. An update may leave it out,
// so the two modes submit different request types.
type OAuth2AppFormSubmit =
	| {
			app?: undefined;
			onSubmit: (
				data: TypesGen.PostOAuth2ProviderAppRequest,
			) => void | Promise<void>;
	  }
	| {
			app: TypesGen.OAuth2ProviderApp;
			onSubmit: (
				data: TypesGen.PutOAuth2ProviderAppRequest,
			) => void | Promise<void>;
	  };

type OAuth2AppFormProps = OAuth2AppFormSubmit & {
	// Passed on its own because the create page has no app yet. The client
	// type decides which redirect URI rules apply.
	clientType: TypesGen.OAuth2ClientType;
	error?: unknown;
	isUpdating: boolean;
	defaultValues?: Partial<OAuth2AppFormValues>;
	disabled: boolean;
	onIconChange?: (icon: string) => void;
};

const BACK_HREF = "/deployment/oauth2-provider/apps";
const SCOPE_LABEL = "Allowed scopes";

// Typed so a renamed scope fails to compile instead of silently not matching.
const coderAll: TypesGen.APIKeyScope = "coder:all";

/**
 * narrowsAllowlist reports whether replacing the stored scope allowlist with
 * next can reject a scope the client could previously be granted. It follows
 * the server in ignoring names outside the catalog. It cannot see which scopes
 * cover others beyond coder:all, so it may warn about a change the server
 * would accept, but it never stays quiet about one the server would reject.
 */
export const narrowsAllowlist = (
	stored: string,
	next: readonly string[],
	catalog: readonly string[],
): boolean => {
	if (next.length === 0) {
		return false;
	}
	const nextGrantable = next.filter((scope) => catalog.includes(scope));
	if (nextGrantable.includes(coderAll)) {
		return false;
	}
	if (stored === "") {
		return true;
	}
	const storedGrantable = stored
		.split(" ")
		.filter((scope) => catalog.includes(scope));
	if (storedGrantable.length === 0) {
		return false;
	}
	return storedGrantable.some((scope) => !nextGrantable.includes(scope));
};

// Mirror codersdk.ValidateRedirectURIShape.
// The server remains authoritative for URL syntax differences between parsers.
// oxlint-disable-next-line eslint/no-script-url -- This blocklist rejects the scheme; it is never used as a navigation target.
const DANGEROUS_CALLBACK_SCHEMES = ["javascript:", "data:", "file:", "ftp:"];

const LOOPBACK_HOSTS = ["localhost", "127.0.0.1", "[::1]"];

// A public client is held to RFC 8252 loopback. A confidential client may also
// use a .localhost subdomain, which is how the server draws the line.
const allowsCleartextHTTP = (hostname: string, isPublicClient: boolean) =>
	LOOPBACK_HOSTS.includes(hostname) ||
	(!isPublicClient && hostname.endsWith(".localhost"));

const isValidCallbackURL = (
	value: string | undefined,
	isPublicClient: boolean,
): boolean => {
	if (!value) {
		return false;
	}
	try {
		const url = new URL(value);
		if (url.protocol === "urn:") {
			return url.href === "urn:ietf:wg:oauth:2.0:oob";
		}
		if (DANGEROUS_CALLBACK_SCHEMES.includes(url.protocol)) {
			return false;
		}
		const target = value.slice(value.indexOf(":") + 1);
		if (!target.startsWith("/") || (!url.host && !url.pathname)) {
			return false;
		}
		if (isPublicClient) {
			if (
				value.includes("#") ||
				["mailto:", "tel:", "sms:"].includes(url.protocol)
			) {
				return false;
			}
		}
		if (
			url.protocol === "http:" &&
			!allowsCleartextHTTP(url.hostname, isPublicClient)
		) {
			return false;
		}
		if (
			(url.protocol === "http:" || url.protocol === "https:") &&
			!/^https?:\/\/[^/\\\s]/i.test(value)
		) {
			return false;
		}
		return true;
	} catch {
		return false;
	}
};

const redirectURIsSchema = (isPublicClient: boolean) =>
	Yup.array()
		.of(
			Yup.string()
				.trim()
				.test(
					"redirect-uri-byte-length",
					`A redirect URI cannot be longer than ${OAuth2RedirectURIMaxBytes} UTF-8 bytes.`,
					(value) =>
						new TextEncoder().encode(value).length <= OAuth2RedirectURIMaxBytes,
				)
				.test(
					"valid-redirect-uri",
					"Please enter a valid redirect URI.",
					(value) => isValidCallbackURL(value, isPublicClient),
				),
		)
		// The server also deduplicates on save, so without this the form
		// would submit a URI it never actually saved and mislead the user.
		.test(
			"unique-redirect-uri",
			"This redirect URI is already used by another row.",
			function (uris) {
				const seen = new Set<string>();
				const errors: Yup.ValidationError[] = [];
				for (const [index, uri] of (uris ?? []).entries()) {
					const trimmed = uri?.trim();
					if (!trimmed) {
						continue;
					}
					if (seen.has(trimmed)) {
						errors.push(this.createError({ path: `${this.path}[${index}]` }));
					}
					seen.add(trimmed);
				}
				return errors.length === 0 || new Yup.ValidationError(errors);
			},
		)
		.min(1, "At least one redirect URI is required.")
		.max(
			OAuth2RedirectURIsMaxCount,
			`At most ${OAuth2RedirectURIsMaxCount} redirect URIs are allowed.`,
		);

const validationSchema = (isPublicClient: boolean) =>
	Yup.object({
		name: Yup.string()
			.trim()
			.required("Please enter a name.")
			.test(
				"name-byte-length",
				`Name cannot be longer than ${OAuth2AppNameMaxBytes} UTF-8 bytes.`,
				(value) =>
					new TextEncoder().encode(value).length <= OAuth2AppNameMaxBytes,
			),
		redirect_uris: redirectURIsSchema(isPublicClient),
		icon: iconValidator,
	});

// app and onSubmit stay together in submit so that checking app also
// narrows the type of onSubmit.
export const OAuth2AppForm: FC<OAuth2AppFormProps> = ({
	clientType,
	error,
	isUpdating,
	defaultValues,
	disabled,
	onIconChange,
	...submit
}) => {
	const { app } = submit;
	const didSubmit = useRef(false);
	const isPublicClient = clientType === "public";
	// A stored list that no longer passes validation disables Update on load.
	// Show its errors right away instead of waiting for the field to be
	// touched, so the admin can see what to correct.
	const storedRedirectURIsInvalid =
		app !== undefined &&
		!redirectURIsSchema(isPublicClient).isValidSync(app.redirect_uris);
	const form = useFormik<OAuth2AppFormValues>({
		initialValues: {
			name: app?.name ?? defaultValues?.name ?? "",
			redirect_uris: app
				? [...app.redirect_uris]
				: defaultValues?.redirect_uris?.length
					? [...defaultValues.redirect_uris]
					: [""],
			icon: app?.icon ?? defaultValues?.icon ?? "",
			scope:
				app?.scope.split(" ").filter(Boolean) ?? defaultValues?.scope ?? [],
		},
		initialTouched: storedRedirectURIsInvalid
			? { redirect_uris: true }
			: undefined,
		validationSchema: validationSchema(isPublicClient),
		validateOnMount: true,
		onSubmit: async ({
			scope: selectedScopes,
			redirect_uris: enteredRedirectURIs,
			...values
		}) => {
			didSubmit.current = true;
			const redirectURIs = enteredRedirectURIs.map((uri) => uri.trim());
			const scope = selectedScopes.join(" ");
			// An untouched allowlist is left out of updates rather than echoed
			// back. The form cannot round-trip a stored list exactly: a whitespace
			// only list grants nothing but would resend as "" and lift the
			// restriction, and a legacy list may exceed the current size limits.
			const scopeChanged = !app || scope !== form.initialValues.scope.join(" ");
			const request = {
				...values,
				name: values.name.trim(),
				...(scopeChanged ? { scope } : {}),
			};
			if (submit.app === undefined) {
				await submit.onSubmit({ ...request, redirect_uris: redirectURIs });
				return;
			}
			// Leave an unchanged list out so an old copy can't undo another
			// admin's edit. Compare with the form's starting list, not the app
			// prop, which refreshes after each save. Trim it because a save
			// resets it to the untrimmed input.
			const loaded = form.initialValues.redirect_uris.map((uri) => uri.trim());
			const redirectURIsChanged =
				redirectURIs.length !== loaded.length ||
				redirectURIs.some((uri, index) => uri !== loaded[index]);
			await submit.onSubmit(
				redirectURIsChanged
					? { ...request, redirect_uris: redirectURIs }
					: request,
			);
		},
	});
	const scopesQuery = useQuery(getExternalScopes());
	const getFieldHelpers = getFormHelpers(form, error);
	const iconField = getFieldHelpers("icon");
	const redirectURIsField = getFieldHelpers("redirect_uris");
	const redirectURIsErrors = form.errors.redirect_uris;
	const redirectURIsRowErrors =
		form.touched.redirect_uris && Array.isArray(redirectURIsErrors)
			? redirectURIsErrors
			: undefined;
	const formDisabled = disabled || isUpdating;
	const editing = Boolean(app);
	// The Scopes section of docs/admin/integrations/oauth2-provider.md explains
	// why narrowing a self-registered application's allowlist can break it.
	const narrowsSelfRegisteredScopes =
		app?.dynamically_registered === true &&
		scopesQuery.data !== undefined &&
		narrowsAllowlist(app.scope, form.values.scope, scopesQuery.data.external);
	const submitDisabled =
		formDisabled || !form.isValid || (editing && !form.dirty);

	// When the parent's mutation finishes without an error, treat the just-
	// submitted values as the new baseline so the unsaved-changes prompt does
	// not fire on subsequent navigations.
	const previousIsUpdating = useRef(isUpdating);
	useEffect(() => {
		if (previousIsUpdating.current && !isUpdating) {
			if (didSubmit.current && !error) {
				form.resetForm({ values: form.values });
			}
			didSubmit.current = false;
		}
		previousIsUpdating.current = isUpdating;
	}, [isUpdating, error, form]);

	const unsavedChanges = useUnsavedChangesPrompt(
		form.dirty && !form.isSubmitting,
	);

	const handleIconChange = (value: string) => {
		void form.setFieldValue("icon", value);
		void form.setFieldTouched("icon", true);
		onIconChange?.(value);
	};

	const handleRedirectURIsChange = (values: string[]) => {
		void form.setFieldValue("redirect_uris", values);
		// Removing the last row disables Update without a blur event to mark
		// the field touched, so the required-URI message never shows. Mark it
		// touched here (skipping its own validate call, since setFieldValue
		// above already triggers one) only for that case, since doing this on
		// every keystroke would race with the blur-triggered validateForm below.
		if (values.length === 0) {
			void form.setFieldTouched("redirect_uris", true, false);
		}
	};
	// Touched is set on blur rather than on every keystroke. Setting it in
	// handleRedirectURIsChange as well would fire a second concurrent
	// validateForm per keystroke; Formik does not guarantee those resolve in
	// order, so the result of an earlier keystroke can overwrite the latest.
	const handleRedirectURIsBlur = () => {
		void form.setFieldTouched("redirect_uris", true);
	};
	// The overall-list message (min/max count, or a server validation error)
	// is shown once below the rows; a per-row message is shown on its row
	// instead, so this excludes the case where Yup reports one message per row.
	const showRedirectURIsListError =
		redirectURIsField.error && !Array.isArray(redirectURIsErrors);

	return (
		<Form onSubmit={form.handleSubmit}>
			<FormFields>
				{Boolean(error) && <ErrorAlert error={error} />}
				<FormField
					field={getFieldHelpers("name")}
					label="Name"
					description="The name of your Coder app."
					disabled={formDisabled}
					autoFocus
					required
				/>
				<div className="flex flex-col gap-2">
					<RedirectURIsField
						values={form.values.redirect_uris}
						onChange={handleRedirectURIsChange}
						onBlur={handleRedirectURIsBlur}
						disabled={formDisabled}
						isPublicClient={isPublicClient}
						errors={redirectURIsRowErrors}
					/>
					{showRedirectURIsListError && (
						<span className="text-xs text-content-destructive">
							{redirectURIsField.helperText}
						</span>
					)}
				</div>
				<div className="flex flex-col gap-2">
					<Label htmlFor="icon">Icon</Label>
					<div className="text-xs text-content-secondary">
						Optional. URL or emoji shown for this application.
					</div>
					<IconField
						id="icon"
						value={form.values.icon}
						disabled={formDisabled}
						label={null}
						onChange={(event) => handleIconChange(event.target.value)}
						onPickEmoji={handleIconChange}
					/>
					{iconField.error ? (
						<span className="text-xs text-content-destructive">
							{iconField.helperText}
						</span>
					) : (
						iconField.helperText && (
							<span className="text-xs text-content-secondary">
								{iconField.helperText}
							</span>
						)
					)}
				</div>

				<div className="flex flex-col gap-2">
					<Label>{SCOPE_LABEL}</Label>
					<div className="text-xs text-content-secondary">
						Optional. Limits the scopes this application's tokens can be
						granted. Empty means no restriction: tokens can be granted any
						scope.
					</div>
					{narrowsSelfRegisteredScopes && (
						<Alert severity="warning">
							A self-registered client that requests every advertised scope
							fails new authorizations with invalid_scope once its allowlist is
							narrower than that list. Existing tokens keep their scopes.{" "}
							<Link
								href={docs("/admin/integrations/oauth2-provider#scopes")}
								target="_blank"
								rel="noreferrer"
							>
								Learn more
								<span className="sr-only"> (link opens in new tab)</span>
							</Link>
						</Alert>
					)}
					<MultiSelectCombobox
						// cmdk generates the input's id and aria-labelledby itself, so
						// the accessible name has to come from its own label prop.
						commandProps={{ label: SCOPE_LABEL }}
						value={form.values.scope.map((scope) => ({
							value: scope,
							label: scope,
						}))}
						options={
							scopesQuery.data?.external.map((scope) => ({
								value: scope,
								label: scope,
							})) ?? []
						}
						onChange={(options) => {
							void form.setFieldValue(
								"scope",
								options.map((option) => option.value),
							);
						}}
						disabled={
							formDisabled || scopesQuery.isLoading || scopesQuery.isError
						}
						hidePlaceholderWhenSelected={!scopesQuery.isLoading}
						placeholder={
							scopesQuery.isLoading ? "Loading scopes..." : "Select scopes"
						}
						emptyIndicator={
							<p className="text-center text-md text-content-primary">
								No matching scopes
							</p>
						}
					/>
					{scopesQuery.isError && (
						<div className="flex items-center gap-3">
							<span className="text-xs text-content-destructive">
								{getErrorMessage(
									scopesQuery.error,
									"Failed to load the list of scopes.",
								)}
							</span>
							<Button
								type="button"
								variant="outline"
								size="xs"
								disabled={scopesQuery.isFetching}
								onClick={() => void scopesQuery.refetch()}
							>
								<Spinner loading={scopesQuery.isFetching} />
								Retry
							</Button>
						</div>
					)}
				</div>

				<div className="flex justify-end gap-4">
					<Button variant="outline" asChild>
						<RouterLink to={BACK_HREF}>Cancel</RouterLink>
					</Button>
					<Button disabled={submitDisabled} type="submit">
						<Spinner loading={isUpdating} />
						{app ? "Update application" : "Create application"}
					</Button>
				</div>
			</FormFields>
			<ConfirmDialog
				type="info"
				hideCancel={false}
				open={unsavedChanges.isOpen}
				onClose={unsavedChanges.onCancel}
				onConfirm={unsavedChanges.onConfirm}
				title="Unsaved changes"
				confirmText="Confirm"
				description={
					<div className="flex items-start gap-3">
						<TriangleAlertIcon className="size-icon-sm mt-1 shrink-0" />
						<p className="m-0">
							Your updates haven't been saved. Leave anyway?
						</p>
					</div>
				}
			/>
		</Form>
	);
};
