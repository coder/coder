import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import MenuItem from "@material-ui/core/MenuItem"
import Select from "@material-ui/core/Select"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import TextField from "@material-ui/core/TextField"
import PersonAdd from "@material-ui/icons/PersonAdd"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { TemplateUser } from "api/typesGenerated"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { EmptyState } from "components/EmptyState/EmptyState"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { Stack } from "components/Stack/Stack"
import { TableLoader } from "components/TableLoader/TableLoader"
import { FC, useState } from "react"

export interface TemplateCollaboratorsPageViewProps {
  deleteTemplateError: Error | unknown
  templateUsers: TemplateUser[] | undefined
}

export const TemplateCollaboratorsPageView: FC<
  React.PropsWithChildren<TemplateCollaboratorsPageViewProps>
> = ({ deleteTemplateError, templateUsers }) => {
  const styles = useStyles()
  const [open, setOpen] = useState(false)
  const [options, setOptions] = useState([])
  const isLoading = false
  const deleteError = deleteTemplateError ? (
    <ErrorSummary error={deleteTemplateError} dismissible />
  ) : null

  return (
    <Stack spacing={2.5}>
      {deleteError}
      <Stack direction="row" alignItems="center" spacing={1}>
        <Autocomplete
          id="asynchronous-demo"
          style={{ width: 300 }}
          open={open}
          onOpen={() => {
            setOpen(true)
          }}
          onClose={() => {
            setOpen(false)
          }}
          getOptionSelected={(option: any, value: any) => option.name === value.name}
          getOptionLabel={(option) => option.name}
          options={options}
          loading={isLoading}
          className={styles.autocomplete}
          renderInput={(params) => (
            <TextField
              {...params}
              margin="none"
              variant="outlined"
              placeholder="User email or username"
              InputProps={{
                ...params.InputProps,
                endAdornment: (
                  <>
                    {isLoading ? <CircularProgress size={16} /> : null}
                    {params.InputProps.endAdornment}
                  </>
                ),
              }}
            />
          )}
        />

        <Select defaultValue="read" variant="outlined" className={styles.select}>
          <MenuItem key="read" value="read">
            Read
          </MenuItem>
          <MenuItem key="write" value="write">
            Write
          </MenuItem>
          <MenuItem key="admin" value="admin">
            Admin
          </MenuItem>
        </Select>

        <Button size="small" startIcon={<PersonAdd />}>
          Add collaborator
        </Button>
      </Stack>

      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>User</TableCell>
              <TableCell>Role</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <ChooseOne>
              <Cond condition={!templateUsers}>
                <TableLoader />
              </Cond>
              <Cond condition={Boolean(templateUsers && templateUsers.length === 0)}>
                <TableRow>
                  <TableCell colSpan={999}>
                    <EmptyState
                      message="No collaborators yet"
                      description="Add a collaborator using the controls above"
                    />
                  </TableCell>
                </TableRow>
              </Cond>
              <Cond condition={Boolean(templateUsers && templateUsers.length > 0)}>
                <TableRow>
                  <TableCell>Kyle</TableCell>
                  <TableCell>Admin</TableCell>
                </TableRow>
              </Cond>
            </ChooseOne>
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  )
}

export const useStyles = makeStyles((theme) => {
  return {
    autocomplete: {
      "& .MuiInputBase-root": {
        width: 300,
        // Match button small height
        height: 36,
      },

      "& input": {
        fontSize: 14,
        padding: `${theme.spacing(0, 0.5, 0, 0.5)} !important`,
      },
    },

    select: {
      // Match button small height
      height: 36,
      fontSize: 14,
      width: 100,
    },
  }
})
