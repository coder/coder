import Button from "@mui/material/Button"
import TextField from "@mui/material/TextField"
import { makeStyles } from "@mui/styles"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Fieldset } from "components/DeploySettingsLayout/Fieldset"
import { Header } from "components/DeploySettingsLayout/Header"
import { FileUpload } from "components/FileUpload/FileUpload"
import KeyboardArrowLeft from "@mui/icons-material/KeyboardArrowLeft"
import { displayError } from "components/GlobalSnackbar/utils"
import { Stack } from "components/Stack/Stack"
import { DividerWithText } from "pages/DeploySettingsPage/LicensesSettingsPage/DividerWithText"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"

type AddNewLicenseProps = {
  onSaveLicenseKey: (license: string) => void
  isSavingLicense: boolean
  savingLicenseError?: unknown
}

export const AddNewLicensePageView: FC<AddNewLicenseProps> = ({
  onSaveLicenseKey,
  isSavingLicense,
  savingLicenseError,
}) => {
  const styles = useStyles()

  function handleFileUploaded(files: File[]) {
    const fileReader = new FileReader()
    fileReader.onload = () => {
      const licenseKey = fileReader.result as string

      onSaveLicenseKey(licenseKey)

      fileReader.onerror = () => {
        displayError("Failed to read file")
      }
    }

    fileReader.readAsText(files[0])
  }

  const isUploading = false

  function onUpload(file: File) {
    handleFileUploaded([file])
  }

  return (
    <>
      <Stack
        alignItems="baseline"
        direction="row"
        justifyContent="space-between"
      >
        <Header
          title="Add a License"
          description="Get access to high availability, RBAC, quotas, and more."
        />
        <Button
          component={RouterLink}
          startIcon={<KeyboardArrowLeft />}
          to="/settings/deployment/licenses"
        >
          All Licenses
        </Button>
      </Stack>

      {savingLicenseError && (
        <AlertBanner severity="error" error={savingLicenseError}></AlertBanner>
      )}

      <FileUpload
        isUploading={isUploading}
        onUpload={onUpload}
        removeLabel="Remove File"
        title="Upload Your License"
        description="Select a text file that contains your license key."
      />

      <Stack className={styles.main}>
        <DividerWithText>or</DividerWithText>

        <Fieldset
          title="Paste Your License"
          onSubmit={(e) => {
            e.preventDefault()

            const form = e.target
            const formData = new FormData(form as HTMLFormElement)

            const licenseKey = formData.get("licenseKey")

            onSaveLicenseKey(licenseKey?.toString() || "")
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
            rows={1}
            fullWidth
          />
        </Fieldset>
      </Stack>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  main: {
    paddingTop: theme.spacing(5),
  },
}))
