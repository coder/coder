import { useFormik } from "formik";
import { TriangleAlertIcon } from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { Link } from "react-router";
import * as Yup from "yup";
import type * as TypesGen from "#/api/typesGenerated";
import {
	OAuth2AppNameMaxBytes,
	OAuth2RedirectURIMaxBytes,
	OAuth2RedirectURIsMaxCount,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Form, FormFields } from "#/components/Form/Form";
import { FormField } from "#/components/FormField/FormField";
import { IconField } from "#/components/IconField/IconField";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { useUnsavedChangesPrompt } from "#/hooks/useUnsavedChangesPrompt";
import { getFormHelpers, iconValidator } from "#/utils/formUtils";
import { RedirectURIsField } from "./RedirectURIsField";

type OAuth2AppFormValues = {
	name: string;
	redirect_uris: string[];
	icon: string;
};

type OAuth2AppFormProps = {
	app?: TypesGen.OAuth2ProviderApp;
	onSubmit: (data: OAuth2AppFormValues) => void | Promise<void>;
	error?: unknown;
	isUpdating: boolean;
	defaultValues?: OAuth2AppFormValues;
	disabled: boolean;
	onIconChange?: (icon: string) => void;
};

const BACK_HREF = "/deployment/oauth2-provider/apps";

// Mirror codersdk.ValidateRedirectURIShape.
// The server remains authoritative for URL syntax differences between parsers.
// oxlint-disable-next-line eslint/no-script-url -- This blocklist rejects the scheme; it is never used as a navigation target.
const DANGEROUS_CALLBACK_SCHEMES = ["javascript:", "data:", "file:", "ftp:"];

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
			if (
				url.protocol === "http:" &&
				!["localhost", "127.0.0.1", "[::1]"].includes(url.hostname)
			) {
				return false;
			}
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
		redirect_uris: Yup.array()
			.of(
				Yup.string()
					.test(
						"redirect-uri-byte-length",
						`A redirect URI cannot be longer than ${OAuth2RedirectURIMaxBytes} UTF-8 bytes.`,
						(value) =>
							new TextEncoder().encode(value).length <=
							OAuth2RedirectURIMaxBytes,
					)
					.test(
						"valid-redirect-uri",
						"Please enter a valid redirect URI.",
						(value) => isValidCallbackURL(value, isPublicClient),
					),
			)
			.min(1, "At least one redirect URI is required.")
			.max(
				OAuth2RedirectURIsMaxCount,
				`At most ${OAuth2RedirectURIsMaxCount} redirect URIs are allowed.`,
			),
		icon: iconValidator,
	});

export const OAuth2AppForm: FC<OAuth2AppFormProps> = ({
	app,
	onSubmit,
	error,
	isUpdating,
	defaultValues,
	disabled,
	onIconChange,
}) => {
	const didSubmit = useRef(false);
	const isPublicClient = app?.client_type === "public";
	const form = useFormik<OAuth2AppFormValues>({
		initialValues: {
			name: app?.name ?? defaultValues?.name ?? "",
			redirect_uris: app
				? [...app.redirect_uris]
				: defaultValues?.redirect_uris?.length
					? [...defaultValues.redirect_uris]
					: [""],
			icon: app?.icon ?? defaultValues?.icon ?? "",
		},
		validationSchema: validationSchema(isPublicClient),
		validateOnMount: true,
		onSubmit: async (values) => {
			didSubmit.current = true;
			const redirectURIs = values.redirect_uris
				.map((uri) => uri.trim())
				.filter(Boolean);
			await onSubmit({
				...values,
				name: values.name.trim(),
				redirect_uris: Array.from(new Set(redirectURIs)),
			});
		},
	});
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

				<div className="flex justify-end gap-4">
					<Button variant="outline" asChild>
						<Link to={BACK_HREF}>Cancel</Link>
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
