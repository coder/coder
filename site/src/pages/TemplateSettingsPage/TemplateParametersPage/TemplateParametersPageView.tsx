import type { FC } from "react";
import { useRef, useState } from "react";
import type { TemplateVersion } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { Label } from "#/components/Label/Label";
import { Link } from "#/components/Link/Link";
import { Separator } from "#/components/Separator/Separator";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StackLabel,
	StackLabelHelperText,
} from "#/components/StackLabel/StackLabel";
import { docs } from "#/utils/docs";
import { formatDate } from "#/utils/time";

interface TemplateParametersPageViewProps {
	activeVersion: TemplateVersion;
	useClassicParameterFlow: boolean;
	canUpdate: boolean;
	isSaving: boolean;
	isRefreshing: boolean;
	error?: unknown;
	onChangeClassicParameterFlow: (useClassicParameterFlow: boolean) => void;
	onRefresh: () => void;
}

export const TemplateParametersPageView: FC<
	TemplateParametersPageViewProps
> = ({
	activeVersion,
	useClassicParameterFlow,
	canUpdate,
	isSaving,
	isRefreshing,
	error,
	onChangeClassicParameterFlow,
	onRefresh,
}) => {
	const [isConfirmingRefresh, setIsConfirmingRefresh] = useState(false);
	const refreshButtonRef = useRef<HTMLButtonElement>(null);
	const importedAt = activeVersion.job.completed_at;

	return (
		<div className="flex max-w-prose flex-col gap-8">
			<SettingsHeader>
				<SettingsHeaderTitle>Parameters</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Control how this template's parameters are resolved when a workspace
					is created.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{error ? <ErrorAlert error={error} /> : null}

			{/*
			 * A disabled control takes no focus and no pointer events, so it cannot
			 * carry the reason it is disabled. Stating the reason here puts it in
			 * reading order ahead of both controls.
			 */}
			{!canUpdate && (
				<p className="m-0 text-sm text-content-secondary">
					You need permission to update this template to change these settings.
				</p>
			)}

			<div className="flex items-start">
				<Checkbox
					id="use_classic_parameter_flow"
					name="use_classic_parameter_flow"
					checked={!useClassicParameterFlow}
					onCheckedChange={(checked) => {
						onChangeClassicParameterFlow(checked !== true);
					}}
					disabled={!canUpdate || isSaving}
				/>
				<Label htmlFor="use_classic_parameter_flow">
					<StackLabel>
						Enable dynamic parameters for workspace creation (recommended)
						<StackLabelHelperText>
							<span>
								The dynamic workspace form allows you to design your template
								with additional form types and identity-aware conditional
								parameters. This is the default option for new templates. The
								classic workspace creation flow will be deprecated in a future
								release.
							</span>
							<Link
								className="text-xs"
								href={docs(
									"/admin/templates/extending-templates/dynamic-parameters",
								)}
							>
								Learn more
							</Link>
						</StackLabelHelperText>
					</StackLabel>
				</Label>
				<Spinner size="sm" className="mt-0.5" loading={isSaving} />
			</div>

			<Separator className="my-2" />

			<section className="flex flex-col gap-4">
				<SettingsHeaderTitle level="h2" hierarchy="secondary">
					Template data
				</SettingsHeaderTitle>

				<p className="m-0 text-sm text-content-secondary">
					Terraform <code>data</code> sources are read once, while a template
					version is being imported, and every workspace built from that version
					reuses the same results. Refreshing imports the active version's
					source again, so the data is read fresh. It also gives the version the
					metadata that dynamic parameters needs.
				</p>

				<dl className="m-0 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
					<dt className="text-content-secondary">Active version</dt>
					<dd className="m-0">{activeVersion.name}</dd>
					<dt className="text-content-secondary">Last imported</dt>
					<dd className="m-0">
						{importedAt ? formatDate(new Date(importedAt)) : "Unknown"}
					</dd>
				</dl>

				<div className="pt-2">
					<Button
						ref={refreshButtonRef}
						disabled={!canUpdate || isRefreshing}
						onClick={() => setIsConfirmingRefresh(true)}
					>
						<Spinner loading={isRefreshing} />
						Refresh template data
					</Button>
				</div>
			</section>

			<ConfirmDialog
				open={isConfirmingRefresh}
				type="info"
				hideCancel={false}
				title="Refresh template data"
				description={
					<>
						This creates a new template version from the same source as{" "}
						<strong>{activeVersion.name}</strong> and makes it the active
						version. New workspaces will use it immediately. Existing workspaces
						keep running until they are updated.
					</>
				}
				confirmText="Refresh"
				onClose={() => setIsConfirmingRefresh(false)}
				onConfirm={() => {
					setIsConfirmingRefresh(false);
					onRefresh();
				}}
				// Radix returns focus to its trigger on close, and this dialog has
				// none, so focus would land on <body>.
				onCloseAutoFocus={(event) => {
					event.preventDefault();
					refreshButtonRef.current?.focus();
				}}
			/>
		</div>
	);
};
