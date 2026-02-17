import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { FileUpload } from "components/FileUpload/FileUpload";
import { displayError } from "components/GlobalSnackbar/utils";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Textarea } from "components/Textarea/Textarea";
import { ChevronLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { Fieldset } from "../Fieldset";
import { DividerWithText } from "./DividerWithText";

type AddNewLicenseProps = {
	onSaveLicenseKey: (license: string) => void;
	isSavingLicense: boolean;
	savingLicenseError?: unknown;
};

export const AddNewLicensePageView: FC<AddNewLicenseProps> = ({
	onSaveLicenseKey,
	isSavingLicense,
	savingLicenseError,
}) => {
	function handleFileUploaded(files: File[]) {
		const fileReader = new FileReader();
		fileReader.onload = () => {
			const licenseKey = fileReader.result as string;

			onSaveLicenseKey(licenseKey);

			fileReader.onerror = () => {
				displayError("Failed to read file");
			};
		};

		fileReader.readAsText(files[0]);
	}

	const isUploading = false;

	function onUpload(file: File) {
		handleFileUploaded([file]);
	}

	return (
		<>
			<div className="flex flex-row items-baseline justify-between">
				<SettingsHeader>
					<SettingsHeaderTitle>Add a license</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Get access to high availability, RBAC, quotas, and more.
					</SettingsHeaderDescription>
				</SettingsHeader>

				<Button asChild variant="outline">
					<RouterLink to="/deployment/licenses">
						<ChevronLeftIcon />
						All Licenses
					</RouterLink>
				</Button>
			</div>

			{savingLicenseError && <ErrorAlert error={savingLicenseError} />}

			<FileUpload
				isUploading={isUploading}
				onUpload={onUpload}
				removeLabel="Remove File"
				title="Upload Your License"
				description="Select a text file that contains your license key."
			/>

			<div className="flex flex-col gap-4 pt-10">
				<DividerWithText>or</DividerWithText>

				<Fieldset
					title="Paste Your License"
					onSubmit={(e) => {
						e.preventDefault();

						const form = e.target;
						const formData = new FormData(form as HTMLFormElement);

						const licenseKey = formData.get("licenseKey");

						onSaveLicenseKey(licenseKey?.toString() || "");
					}}
					button={
						<Button type="submit" disabled={isSavingLicense}>
							Upload License
						</Button>
					}
				>
					<Textarea
						name="licenseKey"
						placeholder="Enter your license..."
						rows={3}
						className="w-full"
					/>
				</Fieldset>
			</div>
		</>
	);
};
