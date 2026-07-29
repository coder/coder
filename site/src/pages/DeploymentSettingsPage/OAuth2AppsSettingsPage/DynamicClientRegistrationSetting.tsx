import { type FC, useId, useState } from "react";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";

type DynamicClientRegistrationSettingProps = {
	enabled: boolean;
	canEdit: boolean;
	onChange: (enabled: boolean) => void;
};

export const DynamicClientRegistrationSetting: FC<
	DynamicClientRegistrationSettingProps
> = ({ enabled, canEdit, onChange }) => {
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
						disabled={!canEdit}
						onClick={() => onChange(false)}
					>
						Disable
					</Button>
				) : (
					<Button
						disabled={!canEdit}
						onClick={() => setIsEnableDialogOpen(true)}
					>
						Enable
					</Button>
				)}
			</section>

			<Dialog open={isEnableDialogOpen} onOpenChange={setIsEnableDialogOpen}>
				<DialogContent variant="destructive" className="max-w-xl">
					<DialogHeader>
						<DialogTitle>Enable Dynamic Client Registration?</DialogTitle>
						<DialogDescription>
							Only enable Dynamic Client Registration if you intend to support
							self-service OAuth2 client registration.
						</DialogDescription>
					</DialogHeader>
					<DialogFooter>
						<DialogActions
							confirmVariant="destructive"
							onConfirm={() => {
								setIsEnableDialogOpen(false);
								onChange(true);
							}}
							onCancel={() => setIsEnableDialogOpen(false)}
						/>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</>
	);
};
