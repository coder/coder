import { makeStyles } from "@mui/styles"
import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import { DeploymentOption } from "api/types"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import { FC } from "react"
import { intervalToDuration, formatDuration } from "date-fns"

const OptionsTable: FC<{
  options: DeploymentOption[]
}> = ({ options }) => {
  const styles = useStyles()

  if (options.length === 0) {
    return <p>No options to configure</p>
  }

  return (
    <TableContainer>
      <Table className={styles.table}>
        <TableHead>
          <TableRow>
            <TableCell width="50%">Option</TableCell>
            <TableCell width="50%">Value</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {Object.values(options).map((option) => {
            if (
              option.value === null ||
              option.value === "" ||
              option.value === undefined
            ) {
              return null
            }
            return (
              <TableRow key={option.flag}>
                <TableCell>
                  <OptionName>{option.name}</OptionName>
                  <OptionDescription>{option.description}</OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>{optionValue(option)}</OptionValue>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </TableContainer>
  )
}

// optionValue is a helper function to format the value of a specific deployment options
export function optionValue(option: DeploymentOption) {
  switch (option.name) {
    case "Max Token Lifetime":
    case "Session Duration":
      // intervalToDuration takes ms, so convert nanoseconds to ms
      return formatDuration(
        intervalToDuration({ start: 0, end: (option.value as number) / 1e6 }),
      )
    case "Strict-Transport-Security":
      if (option.value === 0) {
        return "Disabled"
      }
      return (option.value as number).toString() + "s"
    case "OIDC Group Mapping":
      return Object.entries(option.value as Record<string, string>).map(
        ([key, value]) => `"${key}"->"${value}"`,
      )
    default:
      return option.value
  }
}

const useStyles = makeStyles((theme) => ({
  table: {
    "& td": {
      paddingTop: theme.spacing(3),
      paddingBottom: theme.spacing(3),
    },

    "& td:last-child, & th:last-child": {
      paddingLeft: theme.spacing(4),
    },
  },
}))

export default OptionsTable
