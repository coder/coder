import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableHead from "@mui/material/TableHead";
import TableRow from "@mui/material/TableRow";
import { type FC } from "react";
import Box from "@mui/material/Box";
import { css } from "@emotion/react";
import type { ClibaseOption } from "api/typesGenerated";
import {
  OptionConfig,
  OptionConfigFlag,
  OptionDescription,
  OptionName,
  OptionValue,
} from "components/DeploySettingsLayout/Option";
import { optionValue } from "./optionValue";

const OptionsTable: FC<{
  options: ClibaseOption[];
  additionalValues?: string[];
}> = ({ options, additionalValues }) => {
  if (options.length === 0) {
    return <p>No options to configure</p>;
  }

  return (
    <TableContainer>
      <Table
        css={css`
          & td {
            padding-top: 24px;
            padding-bottom: 24px;
          }

          & td:last-child,
          & th:last-child {
            padding-left: 32px;
          }
        `}
      >
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
                      <OptionConfig source={option.value_source === "flag"}>
                        <OptionConfigFlag>CLI</OptionConfigFlag>
                        --{option.flag}
                      </OptionConfig>
                    )}
                    {option.flag_shorthand && (
                      <OptionConfig source={option.value_source === "flag"}>
                        <OptionConfigFlag>CLI</OptionConfigFlag>-
                        {option.flag_shorthand}
                      </OptionConfig>
                    )}
                    {option.env && (
                      <OptionConfig source={option.value_source === "env"}>
                        <OptionConfigFlag>ENV</OptionConfigFlag>
                        {option.env}
                      </OptionConfig>
                    )}
                    {option.yaml && (
                      <OptionConfig source={option.value_source === "yaml"}>
                        <OptionConfigFlag>YAML</OptionConfigFlag>
                        {option.yaml}
                      </OptionConfig>
                    )}
                  </Box>
                </TableCell>

                <TableCell>
                  <OptionValue>
                    {optionValue(option, additionalValues)}
                  </OptionValue>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

export default OptionsTable;
