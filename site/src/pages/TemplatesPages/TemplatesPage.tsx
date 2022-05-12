import Box from "@material-ui/core/Box"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { Link } from "react-router-dom"
import useSWR from "swr"
import * as TypesGen from "../../api/typesGenerated"
import { CodeExample } from "../../components/CodeExample/CodeExample"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { Header } from "../../components/Header/Header"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { TableHeaderRow } from "../../components/TableHeaders/TableHeaders"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import { TableTitle } from "../../components/TableTitle/TableTitle"

export const Language = {
  title: "Templates",
  tableTitle: "All templates",
  nameLabel: "Name",
  emptyMessage: "No templates have been created yet",
  emptyDescription: "Run the following command to get started:",
}

export const TemplatesPage: React.FC = () => {
  const styles = useStyles()
  const { data: orgs, error: orgsError } = useSWR<TypesGen.Organization[], Error>("/api/v2/users/me/organizations")
  const { data: templates, error } = useSWR<TypesGen.Template[] | null, Error>(
    orgs ? `/api/v2/organizations/${orgs[0].id}/templates` : null,
  )
  const isLoading = !templates || !orgs
  const subTitle = templates ? `${templates.length} total` : undefined
  const hasError = orgsError || error
  // Create a dictionary of organization ID -> organization Name
  // Needed to properly construct links to dive into a template
  const orgDictionary =
    orgs &&
    orgs.reduce((acc: Record<string, string>, curr: TypesGen.Organization) => {
      return {
        ...acc,
        [curr.id]: curr.name,
      }
    }, {})

  return (
    <Stack spacing={4}>
      <Header title={Language.title} subTitle={subTitle} />
      <Margins>
        {error && <ErrorSummary error={error} />}
        {orgsError && <ErrorSummary error={orgsError} />}
        {!hasError && (
          <Table>
            <TableHead>
              <TableTitle title={Language.tableTitle} />
              <TableHeaderRow>
                <TableCell size="small">{Language.nameLabel}</TableCell>
              </TableHeaderRow>
            </TableHead>
            <TableBody>
              {isLoading && <TableLoader />}
              {templates &&
                orgs &&
                orgDictionary &&
                templates.map((t) => (
                  <TableRow key={t.id}>
                    <TableCell>
                      <Link to={`/templates/${orgDictionary[t.organization_id]}/${t.name}`}>{t.name}</Link>
                    </TableCell>
                  </TableRow>
                ))}

              {templates && templates.length === 0 && (
                <TableRow>
                  <TableCell colSpan={999}>
                    <Box p={4}>
                      <EmptyState
                        message={Language.emptyMessage}
                        description={
                          <div>
                            <div className={styles.descriptionLabel}>{Language.emptyDescription}</div>
                            <CodeExample code="coder templates create" />
                          </div>
                        }
                      />
                    </Box>
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  descriptionLabel: {
    marginBottom: theme.spacing(1),
  },
}))
