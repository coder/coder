import TextField from "@mui/material/TextField";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Button } from "components/Button/Button";
import { FileUpload } from "components/FileUpload/FileUpload";
import { displayError } from "components/GlobalSnackbar/utils";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "components/SettingsHeader/SettingsHeader";
import { Stack } from "components/Stack/Stack";
import { ChevronLeftIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
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
			<Stack
				alignItems="baseline"
				direction="row"
				justifyContent="space-between"
			>
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
			</Stack>

			{savingLicenseError && <ErrorAlert error={savingLicenseError} />}

			<FileUpload
				isUploading={isUploading}
				onUpload={onUpload}
				removeLabel="Remove File"
				title="Upload Your License"
				description="Select a text file that contains your license key."
			/>

			<Stack css={{ paddingTop: 40 }}>
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
					<TextField
						name="licenseKey"
						placeholder="Enter your license..."
						multiline
						rows={3}
						fullWidth
					/>
				</Fieldset>
			</Stack>
		</>
	);
};
