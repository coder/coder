import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { DeploymentOption } from "api/types"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import { FC } from "react"
import { DisabledBadge } from "./Badges"
import { intervalToDuration, formatDuration } from "date-fns"

const OptionsTable: FC<{
  options: DeploymentOption[]
}> = ({ options }) => {
  const styles = useStyles()

  if (options.length === 0) {
    return <DisabledBadge></DisabledBadge>
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
export function optionValue(
  option: DeploymentOption,
): string[] | string | unknown {
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
