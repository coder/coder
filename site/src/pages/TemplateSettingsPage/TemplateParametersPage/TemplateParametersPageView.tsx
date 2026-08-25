import { useRef, useState } from "react";
import type { TemplateVersion } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
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

type TemplateParametersPageViewProps = {
	activeVersion: TemplateVersion;
	useClassicParameterFlow: boolean;
	canUpdate: boolean;
	isSaving: boolean;
	isRefreshing: boolean;
	error?: unknown;
	onChangeClassicParameterFlow: (useClassicParameterFlow: boolean) => void;
	onRefresh: () => void;
};

export const TemplateParametersPageView: React.FC<
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

			{!canUpdate && (
				<p className="m-0 text-sm text-content-secondary">
					You need permission to update this template to change these settings.
				</p>
			)}

			<div className="flex items-start">
				{/*
				 * Spinner renders its children until it is loading, then replaces
				 * them. At size sm it is 18px, the same as the checkbox, and the
				 * matching margin keeps the label from moving during the swap.
				 */}
				<Spinner
					size="sm"
					className="m-1 shrink-0"
					loading={isSaving}
					label="Saving parameter compatibility mode"
				>
					<Checkbox
						id="use_classic_parameter_flow"
						name="use_classic_parameter_flow"
						checked={useClassicParameterFlow}
						onCheckedChange={(checked) => {
							onChangeClassicParameterFlow(checked === true);
						}}
						disabled={!canUpdate}
					/>
				</Spinner>
				<StackLabel>
					<Label htmlFor="use_classic_parameter_flow">
						<span className="flex flex-row items-center gap-2">
							Use parameter compatibility mode for workspace builds
							<Badge size="sm" variant="warning">
								Deprecated
							</Badge>
						</span>
					</Label>
					<StackLabelHelperText>
						Turn this on only if this template does not work with dynamic
						parameters. Compatibility mode will force workspace builds to fall
						back to the older "Rich parameters" strategy, which is deprecated
						and will be removed in a future release.
					</StackLabelHelperText>
					<StackLabelHelperText>
						Dynamic parameters are the default since Coder 2.25, and let you use
						additional form types, identity-aware parameter values, conditional
						parameters, and data sources. In some cases it may be necessary to
						refresh template data for dynamic parameters to work properly.
					</StackLabelHelperText>
					<Link
						className="self-start text-xs"
						href={docs(
							"/admin/templates/extending-templates/dynamic-parameters",
						)}
					>
						Learn more
					</Link>
				</StackLabel>
			</div>

			<Separator className="my-2" />

			<section className="flex flex-col gap-4">
				<SettingsHeaderTitle level="h2" hierarchy="secondary">
					Template data
				</SettingsHeaderTitle>

				<p className="m-0 text-sm text-content-secondary">
					Coder caches certain values (like Terraform <code>data</code> sources)
					when a new template version is created, and every workspace built from
					that version will reuse the same results. Refreshing will create a new
					template version, allowing cached values to be updated.
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
