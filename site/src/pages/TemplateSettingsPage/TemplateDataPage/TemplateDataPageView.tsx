import type { FC } from "react";
import { useRef, useState } from "react";
import type { TemplateVersion } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { Spinner } from "#/components/Spinner/Spinner";
import { formatDate } from "#/utils/time";

interface TemplateDataPageViewProps {
	activeVersion: TemplateVersion;
	canRefresh: boolean;
	isRefreshing: boolean;
	error?: unknown;
	onRefresh: () => void;
}

export const TemplateDataPageView: FC<TemplateDataPageViewProps> = ({
	activeVersion,
	canRefresh,
	isRefreshing,
	error,
	onRefresh,
}) => {
	const [isConfirmingRefresh, setIsConfirmingRefresh] = useState(false);
	const refreshButtonRef = useRef<HTMLButtonElement>(null);
	const importedAt = activeVersion.job.completed_at;

	return (
		<div className="flex flex-col gap-6">
			<SettingsHeader>
				<SettingsHeaderTitle>Data</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Terraform <code>data</code> sources are read once, while a template
					version is being imported, and every workspace built from that version
					reuses the same results. Refreshing imports the active version's
					source again, so the data is read fresh.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{error ? <ErrorAlert error={error} /> : null}

			<dl className="m-0 grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
				<dt className="text-content-secondary">Active version</dt>
				<dd className="m-0">{activeVersion.name}</dd>
				<dt className="text-content-secondary">Last imported</dt>
				<dd className="m-0">
					{importedAt ? formatDate(new Date(importedAt)) : "Unknown"}
				</dd>
			</dl>

			{canRefresh && (
				<div>
					<Button
						ref={refreshButtonRef}
						disabled={isRefreshing}
						onClick={() => setIsConfirmingRefresh(true)}
					>
						<Spinner loading={isRefreshing} />
						Refresh template data
					</Button>
				</div>
			)}

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
