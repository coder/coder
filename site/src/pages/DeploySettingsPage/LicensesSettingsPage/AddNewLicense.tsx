
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { Header } from "components/DeploySettingsLayout/Header"

const AddNewLicense: FC = () => {

  return (
    <>
      <Stack alignItems="baseline" direction="row" justifyContent="space-between">
        <Header
          title="Add your license"
          description="Add a license to your account to unlock more features."
        />
      </Stack>

      <Stack spacing={4}>

      </Stack>
    </>
  )
}

export default AddNewLicense
