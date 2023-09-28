import { makeStyles } from "@mui/styles";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import {
  OptionConfig,
  OptionConfigFlag,
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option";
import { FC } from "react";
import { optionValue } from "./optionValue";
import { DeploymentOption } from "api/api";
import Box from "@mui/material/Box";

const OptionsTable: FC<{
  options: DeploymentOption[];
}> = ({ options }) => {
  const styles = useStyles();

  if (options.length === 0) {
    return <p>No options to configure</p>;
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
              return null;
            }
            return (
              <TableRow key={option.flag}>
                <TableCell>
                  <OptionName>{option.name}</OptionName>
                  <OptionDescription>{option.description}</OptionDescription>
                  <Box
                    sx={{
                      marginTop: 3,
                      display: "flex",
                      flexWrap: "wrap",
                      gap: 1,
                    }}
                  >
                    {option.flag && (
                      <OptionConfig>
                        <OptionConfigFlag>CLI</OptionConfigFlag>
                        --{option.flag}
                      </OptionConfig>
                    )}
                    {option.flag_shorthand && (
                      <OptionConfig>
                        <OptionConfigFlag>CLI</OptionConfigFlag>-
                        {option.flag_shorthand}
                      </OptionConfig>
                    )}
                    {option.env && (
                      <OptionConfig>
                        <OptionConfigFlag>ENV</OptionConfigFlag>
                        {option.env}
                      </OptionConfig>
                    )}
                    {option.yaml && (
                      <OptionConfig>
                        <OptionConfigFlag>YAML</OptionConfigFlag>
                        {option.yaml}
                      </OptionConfig>
                    )}
                  </Box>
                </TableCell>

                <TableCell>
                  <OptionValue>{optionValue(option)}</OptionValue>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

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
}));

export default OptionsTable;
