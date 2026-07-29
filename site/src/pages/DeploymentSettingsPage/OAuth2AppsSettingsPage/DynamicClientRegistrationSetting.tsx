import { type FC, useId, useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Spinner } from "#/components/Spinner/Spinner";

type DynamicClientRegistrationSettingProps = {
	enabled: boolean;
	canEdit: boolean;
	isUpdating: boolean;
	onChange: (enabled: boolean) => void;
};

export const DynamicClientRegistrationSetting: FC<
	DynamicClientRegistrationSettingProps
> = ({ enabled, canEdit, isUpdating, onChange }) => {
	const headingId = useId();
	const [isEnableDialogOpen, setIsEnableDialogOpen] = useState(false);

	return (
		<>
			<section
				aria-labelledby={headingId}
				className="flex flex-row items-start justify-between gap-8"
			>
				<div className="flex flex-col gap-1 max-w-xl">
					<div className="flex flex-row items-center gap-2">
						<h3
							id={headingId}
							className="text-content-primary text-base font-semibold m-0"
						>
							Dynamic Client Registration
						</h3>
						{enabled && (
							<Badge size="sm" variant="green" className="border-0 shadow-none">
								Enabled
							</Badge>
						)}
					</div>
					<p className="text-sm text-content-secondary m-0">
						Allow OAuth2 clients to register themselves against this deployment
						without prior administrator approval (RFC 7591).
					</p>
				</div>

				{enabled ? (
					<Button
						variant="outline"
						disabled={!canEdit || isUpdating}
						onClick={() => onChange(false)}
					>
						<Spinner loading={isUpdating} aria-hidden />
						Disable
					</Button>
				) : (
					<Button
						disabled={!canEdit || isUpdating}
						onClick={() => setIsEnableDialogOpen(true)}
					>
						<Spinner loading={isUpdating} aria-hidden />
						Enable
					</Button>
				)}
			</section>

			<ConfirmDialog
				type="delete"
				hideCancel={false}
				open={isEnableDialogOpen}
				onConfirm={() => {
					setIsEnableDialogOpen(false);
					onChange(true);
				}}
				onClose={() => setIsEnableDialogOpen(false)}
				title="Enable Dynamic Client Registration?"
				confirmText="Enable"
				description="Only enable Dynamic Client Registration if you intend to support self-service OAuth2 client registration."
			/>
		</>
	);
};
