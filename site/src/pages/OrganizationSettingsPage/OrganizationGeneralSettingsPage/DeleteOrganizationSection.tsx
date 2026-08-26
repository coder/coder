import { type FC, useState } from "react";
import { Button } from "#/components/Button/Button";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { FormSection, HorizontalForm } from "#/components/Form/Form";

type DeleteOrganizationSectionProps = {
	organizationName: string;
	onDeleteOrganization: () => void;
};

export const DeleteOrganizationSection: FC<DeleteOrganizationSectionProps> = ({
	organizationName,
	onDeleteOrganization,
}) => {
	const [isDeleting, setIsDeleting] = useState(false);

	return (
		<>
			<HorizontalForm className="mt-12">
				<FormSection
					title="Delete Organization"
					description="Delete your organization permanently."
				>
					<div className="flex flex-col gap-4 flex-grow">
						<div className="flex bg-surface-red items-center justify-between border border-solid border-border-destructive rounded-md p-3 pl-4 gap-2">
							<span>Deleting an organization is irreversible.</span>
							<Button
								variant="destructive"
								onClick={() => setIsDeleting(true)}
								className="min-w-fit"
							>
								Delete this organization
							</Button>
						</div>
					</div>
				</FormSection>
			</HorizontalForm>

			<DeleteDialog
				isOpen={isDeleting}
				onConfirm={async () => {
					await onDeleteOrganization();
					setIsDeleting(false);
				}}
				onCancel={() => setIsDeleting(false)}
				entity="organization"
				name={organizationName}
			/>
		</>
	);
};
