import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { DeploymentConfigField, Flaggable } from "api/typesGenerated"
import {
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option"
import { FC } from "react"

const OptionsTable: FC<{
  options: Record<string, DeploymentConfigField<Flaggable>>
}> = ({ options }) => {
  const styles = useStyles()

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
            return (
              <TableRow key={option.flag}>
                <TableCell>
                  <OptionName>{option.name}</OptionName>
                  <OptionDescription>{option.usage}</OptionDescription>
                </TableCell>

                <TableCell>
                  <OptionValue>{option.value.toString()}</OptionValue>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </TableContainer>
  )
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
