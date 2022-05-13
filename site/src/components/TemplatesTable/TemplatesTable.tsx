import Box from "@material-ui/core/Box"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { Link } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { CodeExample } from "../../components/CodeExample/CodeExample"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import { TableHeaderRow } from "../../components/TableHeaders/TableHeaders"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import { TableTitle } from "../../components/TableTitle/TableTitle"

export const Language = {
  title: "Templates",
  tableTitle: "All templates",
  nameLabel: "Name",
  emptyMessage: "No templates have been created yet",
  emptyDescription: "Run the following command to get started:",
  totalLabel: "total",
}

export interface TemplatesTableProps {
  templates?: TypesGen.Template[]
  organizations?: TypesGen.Organization[]
}

export const TemplatesTable: React.FC<TemplatesTableProps> = ({ templates, organizations }) => {
  const isLoading = !templates || !organizations

  // Create a dictionary of organization ID -> organization Name
  // Needed to properly construct links to dive into a template
  const orgDictionary =
    organizations &&
    organizations.reduce((acc: Record<string, string>, curr: TypesGen.Organization) => {
      return {
        ...acc,
        [curr.id]: curr.name,
      }
    }, {})

  return (
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
          organizations &&
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
                  description={Language.emptyDescription}
                  cta={<CodeExample code="coder templates create" />}
                />
              </Box>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  )
}
